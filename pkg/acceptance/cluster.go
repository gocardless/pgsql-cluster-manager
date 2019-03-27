package acceptance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/beevik/etree"
	"github.com/coreos/etcd/clientv3"
	docker "github.com/fsouza/go-dockerclient"
	kitlog "github.com/go-kit/kit/log"
	"github.com/gocardless/pgsql-cluster-manager/pkg/pacemaker"

	. "github.com/onsi/gomega"
)

var networkTimeout = 10 * time.Second // wait for docker to allocate IPs
var startTimeout = 3 * time.Minute    // wait for cluster to define master/sync/async
var etcdDialTimeout = 3 * time.Second // etcd dial timeout

type ClusterOptions struct {
	Workspace   string
	DockerImage string
}

// Cluster wraps three postgres cluster members, providing the Roles method to inspect the
// roles of each node.
type Cluster struct {
	ctx     context.Context
	client  *docker.Client
	members []*docker.Container
}

// Hostname returns the IP of the docker host
func (c *Cluster) Hostname() string {
	url, err := url.Parse(c.client.Endpoint())
	Expect(err).NotTo(
		HaveOccurred(), "failed to parse etcd client endpoint: %s", c.client.Endpoint(),
	)

	// If hostname is empty, we probably have a unix socket, implying we are the docker host
	if hostname := url.Hostname(); hostname != "" {
		return hostname
	}

	return "127.0.0.1"
}

// EtcdClient returns a client connection to the etcd cluster running on the cluster
// members.
func (c *Cluster) EtcdClient() *clientv3.Client {
	member := c.members[0]
	hostPort := member.NetworkSettings.Ports["2379/tcp"][0].HostPort

	client, err := clientv3.New(
		clientv3.Config{
			Endpoints:   []string{fmt.Sprintf("http://%s:%s", c.Hostname(), hostPort)},
			DialTimeout: etcdDialTimeout,
		},
	)

	Expect(err).NotTo(HaveOccurred())
	return client
}

// Shutdown forcibly removes all containers
func (c *Cluster) Shutdown() {
	for _, member := range c.members {
		c.client.RemoveContainer(docker.RemoveContainerOptions{ID: member.ID, Force: true})
	}
}

// Master is a convenience method to provide the current master node
func (c *Cluster) Master() *docker.Container {
	master, _, _ := c.Roles()
	return master
}

// Roles returns a triple of master, sync, async docker containers. When a role doesn't
// exist, the container will be nil.
func (c *Cluster) Roles() (*docker.Container, *docker.Container, *docker.Container) {
	crm := pacemaker.NewPacemaker(c.Executor())
	nodes, err := crm.Get(c.ctx, pacemaker.MasterXPath, pacemaker.SyncXPath, pacemaker.AsyncXPath)

	if err != nil {
		return nil, nil, nil
	}

	lookup := func(node *etree.Element) *docker.Container {
		if node == nil {
			return nil // the cluster doesn't have this member
		}

		if nodeName := node.SelectAttrValue("uname", ""); nodeName != "" {
			for _, member := range c.members {
				if member.Config.Hostname == nodeName {
					return member
				}
			}
		}

		return nil
	}

	return lookup(nodes[0]), lookup(nodes[1]), lookup(nodes[2])
}

// Executor returns a handle to execute commands against a cluster member. It's assumed
// this will be to issue pacemaker commands, and so the caller does not care which member
// the command executes against.
func (c *Cluster) Executor() dockerExecutor {
	return dockerExecutor{c.client, c.members[0]}
}

type dockerExecutor struct {
	client    *docker.Client
	container *docker.Container
}

// CombinedOutput executes a command against the container and will block until it
// terminates, returning the command output as a byte slice.
func (e dockerExecutor) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	var output bytes.Buffer

	exec, err := e.client.CreateExec(docker.CreateExecOptions{
		AttachStderr: true,
		AttachStdout: true,
		Cmd:          append([]string{name}, args...),
		Container:    e.container.ID,
		Privileged:   true,
	})

	if err != nil {
		return nil, err
	}

	err = e.client.StartExec(exec.ID, docker.StartExecOptions{
		Context:      ctx,
		Detach:       false,
		OutputStream: &output,
		ErrorStream:  &output,
	})

	if err != nil {
		return nil, err
	}

	execStatus, err := e.client.InspectExec(exec.ID)
	if execStatus.Running {
		err = errors.New("exec'ed command has not finished")
	}

	if err == nil && execStatus.ExitCode != 0 {
		err = fmt.Errorf("command exited with status %d", execStatus.ExitCode)
	}

	return output.Bytes(), err
}

func StartCluster(ctx context.Context, logger kitlog.Logger, opt ClusterOptions) *Cluster {
	logger.Log("event", "cluster.starting")
	defer func(begin time.Time) {
		logger.Log("event", "cluster.started", "elapsed", time.Since(begin).Seconds())
	}(time.Now())

	client, err := docker.NewClientFromEnv()
	Expect(err).NotTo(HaveOccurred())

	createMember := func(name string) (c *docker.Container) {
		c, err := client.CreateContainer(docker.CreateContainerOptions{
			Context: ctx,
			HostConfig: &docker.HostConfig{
				Binds: []string{
					fmt.Sprintf("%s:/pgsql-cluster-manager", opt.Workspace),
					"/var/run/docker.sock:/var/run/docker.sock",
				},
				Privileged:      true,
				PublishAllPorts: true,
			},
			Config: &docker.Config{
				Hostname:   name,
				Image:      opt.DockerImage,
				Entrypoint: []string{"/usr/bin/dumb-init", "--"},
				Cmd:        []string{"bash", "-c", "while :; do sleep 1; done"},
				ExposedPorts: map[docker.Port]struct{}{
					"6432/tcp": struct{}{}, // PgBouncer
					"2379/tcp": struct{}{}, // etcd
				},
			},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(client.StartContainerWithContext(c.ID, nil, ctx)).To(Succeed())

		Eventually(
			func() (err error) { c, err = client.InspectContainer(c.ID); return },
			networkTimeout,
			250*time.Millisecond,
		).Should(
			Succeed(),
		)

		Expect(c.NetworkSettings).NotTo(BeNil())
		Expect(c.NetworkSettings.IPAddress).NotTo(Equal(""))

		return c
	}

	pg01 := createMember("pg01")
	pg02 := createMember("pg02")
	pg03 := createMember("pg03")

	ids := []string{pg01.ID, pg02.ID, pg03.ID}

	startMember := func(node *docker.Container) {
		logger.Log("event", "member.starting", "name", node.Name)
		defer func(begin time.Time) {
			logger.Log("event", "member.started", "name", node.Name, "elapsed", time.Since(begin).Seconds())
		}(time.Now())

		_, err := dockerExecutor{client, node}.CombinedOutput(ctx, "/bin/start-cluster", ids...)
		Expect(err).NotTo(HaveOccurred())
	}

	var wg sync.WaitGroup
	ready := make(chan struct{}, 1)

	wg.Add(3)

	go func() { startMember(pg01); wg.Done() }()
	go func() { startMember(pg02); wg.Done() }()
	go func() { startMember(pg03); wg.Done() }()

	go func() { wg.Wait(); ready <- struct{}{} }()

	Eventually(ready, startTimeout).Should(
		Receive(), "timed out waiting for cluster to start",
	)

	return &Cluster{ctx: ctx, client: client, members: []*docker.Container{pg01, pg02, pg03}}
}
