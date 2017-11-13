package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/beevik/etree"
	"github.com/coreos/etcd/clientv3"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/gocardless/pgsql-cluster-manager/pacemaker"
	"github.com/stretchr/testify/require"
)

var networkTimeout = 10 * time.Second // wait for docker to allocate IPs
var startTimeout = 3 * time.Minute    // wait for cluster to define master/sync/async

// Cluster wraps three postgres cluster members, providing the Roles method to inspect the
// roles of each node.
type Cluster struct {
	client  *docker.Client
	members []*docker.Container
}

// Hostname returns the IP of the docker host
func (c *Cluster) Hostname(t *testing.T) string {
	url, err := url.Parse(c.client.Endpoint())
	require.Nil(t, err)

	// If hostname is empty, we probably have a unix socket, implying we are the docker host
	if hostname := url.Hostname(); hostname != "" {
		return hostname
	}

	return "127.0.0.1"
}

// EtcdClient returns a client connection to the etcd cluster running on the cluster
// members.
func (c *Cluster) EtcdClient(t *testing.T) *clientv3.Client {
	member := c.members[0]
	hostPort := member.NetworkSettings.Ports["2379/tcp"][0].HostPort

	client, err := clientv3.New(
		clientv3.Config{
			Endpoints:   []string{fmt.Sprintf("http://%s:%s", c.Hostname(t), hostPort)},
			DialTimeout: 3 * time.Second,
		},
	)

	require.Nil(t, err)
	return client
}

// Shutdown forcibly removes all containers
func (c *Cluster) Shutdown() {
	for _, member := range c.members {
		c.client.RemoveContainer(docker.RemoveContainerOptions{ID: member.ID, Force: true})
	}
}

// Roles returns a triple of master, sync, async docker containers. When a role doesn't
// exist, the container will be nil.
func (c *Cluster) Roles(ctx context.Context) (*docker.Container, *docker.Container, *docker.Container) {
	crm := pacemaker.NewPacemaker(pacemaker.WithExecutor(c.Executor()))
	nodes, err := crm.Get(ctx, pacemaker.MasterXPath, pacemaker.SyncXPath, pacemaker.AsyncXPath)

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

	return output.Bytes(), err
}

func StartCluster(t *testing.T, ctx context.Context) *Cluster {
	client, err := docker.NewClientFromEnv()
	if err != nil {
		require.Fail(t, "failed to initialise docker client: %s", err.Error())
	}

	createMember := func(name, workspaceDirectory string) *docker.Container {
		c, err := client.CreateContainer(docker.CreateContainerOptions{
			Context: ctx,
			HostConfig: &docker.HostConfig{
				Binds: []string{
					fmt.Sprintf("%s:/pgsql-cluster-manager", workspaceDirectory),
					"/var/run/docker.sock:/var/run/docker.sock",
				},
				Privileged:      true,
				PublishAllPorts: true,
			},
			Config: &docker.Config{
				Hostname:   name,
				Entrypoint: []string{"/usr/bin/dumb-init", "--"},
				Cmd:        []string{"bash", "-c", "while :; do sleep 1; done"},
				Image:      "gocardless/postgres-member",
				ExposedPorts: map[docker.Port]struct{}{
					"6432/tcp": struct{}{}, // PGBouncer
					"2379/tcp": struct{}{}, // etcd
				},
			},
		})

		require.Nil(t, err)
		require.Nil(t, client.StartContainerWithContext(c.ID, nil, ctx))

		timeout := time.After(networkTimeout)

		for {
			select {
			case <-timeout:
				require.Fail(t, fmt.Sprintf("timed out waiting for container [%s] to receive IP", c.ID))
			default:
				c, err := client.InspectContainer(c.ID)
				require.Nil(t, err)

				if c.NetworkSettings != nil && c.NetworkSettings.IPAddress != "" {
					return c
				}
			}
		}
	}

	workspaceDirectory, found := os.LookupEnv("PGSQL_WORKSPACE")
	require.True(t, found, "test requires PGSQL_WORKSPACE to be set")

	debs, _ := filepath.Glob(fmt.Sprintf("%s/*.deb", workspaceDirectory))
	require.Equal(t, 1, len(debs), "PGSQL_WORKSPACE needs to contain a single .deb")

	pg01 := createMember("pg01", workspaceDirectory)
	pg02 := createMember("pg02", workspaceDirectory)
	pg03 := createMember("pg03", workspaceDirectory)

	ids := []string{pg01.ID, pg02.ID, pg03.ID}

	startMember := func(node *docker.Container) {
		_, err := dockerExecutor{client, node}.CombinedOutput(ctx, "/bin/start-cluster", ids...)
		require.Nil(t, err)
	}

	var wg sync.WaitGroup
	ready := make(chan struct{}, 1)

	wg.Add(3)

	go func() { startMember(pg01); wg.Done() }()
	go func() { startMember(pg02); wg.Done() }()
	go func() { startMember(pg03); wg.Done() }()

	go func() { wg.Wait(); ready <- struct{}{} }()

	select {
	case <-time.After(startTimeout):
		require.Fail(t, "timed out waiting for cluster to start")
	case <-ready:
		// Cluster is ready!
	}

	return &Cluster{client: client, members: []*docker.Container{pg01, pg02, pg03}}
}
