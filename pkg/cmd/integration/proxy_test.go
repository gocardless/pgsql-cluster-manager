package integration

import (
	"context"
	"time"

	"github.com/gocardless/pgsql-cluster-manager/pkg/cmd"
	"github.com/gocardless/pgsql-cluster-manager/pkg/etcd"
	"github.com/gocardless/pgsql-cluster-manager/pkg/pgbouncer"
	"github.com/gocardless/pgsql-cluster-manager/pkg/streams"

	. "github.com/gocardless/pgsql-cluster-manager/pkg/etcd/integration"
	. "github.com/gocardless/pgsql-cluster-manager/pkg/pgbouncer/integration"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("$ pgcm proxy", func() {
	var (
		ctx         context.Context
		cancel      func()
		etcdHostKey string
		proxy       *cmd.ProxyOptions
		bouncer     *pgbouncer.PgBouncer
		cleanup     func()
		err         error
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		bouncer, cleanup, err = StartPgBouncer("postgres", "postgres", "5432")
		Expect(err).NotTo(HaveOccurred())

		// Use a random key in etcd to permit parallel test runs
		etcdHostKey = RandomKey()

		proxy = &cmd.ProxyOptions{
			etcd.StreamOptions{
				Ctx:          ctx,
				GetTimeout:   time.Second,
				PollInterval: 250 * time.Millisecond,
				Keys:         []string{etcdHostKey},
			},
			streams.RetryFoldOptions{
				Ctx:      ctx,
				Interval: 250 * time.Millisecond,
				Timeout:  time.Second,
			},
		}
	})

	AfterEach(func() {
		cancel()
		cleanup()
	})

	put := func(key, value string) {
		_, err := client.Put(ctx, key, value)
		Expect(err).NotTo(HaveOccurred())
	}

	showDatabases := func() ([]pgbouncer.Database, error) {
		return bouncer.ShowDatabases(ctx)
	}

	Context("When etcd key exists", func() {
		It("Configures PgBouncer host in response to changes", func() {
			put(etcdHostKey, "127.0.0.123")
			go proxy.Run(ctx, client, bouncer)

			Eventually(showDatabases).Should(
				ContainElement(
					pgbouncer.Database{
						Name: "postgres",
						Port: "5432",
						Host: "127.0.0.123",
					},
				),
			)

			put(etcdHostKey, "127.0.0.456")

			Eventually(showDatabases).Should(
				ContainElement(
					pgbouncer.Database{
						Name: "postgres",
						Port: "5432",
						Host: "127.0.0.456",
					},
				),
			)
		})
	})
})
