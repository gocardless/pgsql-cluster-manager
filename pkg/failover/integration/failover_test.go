package integration

import (
	"context"
	"time"

	"github.com/coreos/etcd/clientv3/concurrency"
	kitlog "github.com/go-kit/kit/log"
	"github.com/gocardless/pgsql-cluster-manager/pkg/etcd/integration"
	"github.com/gocardless/pgsql-cluster-manager/pkg/failover"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Failover", func() {
	var (
		ctx         context.Context
		logger      kitlog.Logger
		cancel      func()
		etcdHostKey string
		fo          *failover.Failover
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		logger = kitlog.NewLogfmtLogger(GinkgoWriter)
		etcdHostKey = integration.RandomKey()

		session, err := concurrency.NewSession(client)
		Expect(err).NotTo(HaveOccurred(), "failed to create etcd session")
		locker := concurrency.NewMutex(session, etcdHostKey)

		fo = failover.NewFailover(
			logger,
			client,
			map[string]failover.FailoverClient{},
			locker,
			failover.FailoverOptions{
				EtcdHostKey:        etcdHostKey,
				HealthCheckTimeout: time.Second,
				LockTimeout:        time.Second,
				PauseTimeout:       time.Second,
				PauseExpiry:        time.Second,
				ResumeTimeout:      time.Second,
				PacemakerTimeout:   time.Second,
			},
		)
	})

	AfterEach(func() {
		cancel()
	})

	Describe("NotifyWhenMaster", func() {
		Context("When etcd is updated", func() {
			Context("With target address", func() {
				It("Notifies channel", func() {
					go client.Put(ctx, etcdHostKey, "10.0.0.1")

					Eventually(fo.NotifyWhenMaster(ctx, logger, "10.0.0.1")).Should(
						Receive(),
					)
				})
			})

			Context("With non-target address", func() {
				It("Does not notify channel", func() {
					go func() {
						client.Put(ctx, etcdHostKey, "not-target-address")
						client.Put(ctx, etcdHostKey, "and-again")
					}()

					// Wait a full second to ensure updates are propagated
					Consistently(fo.NotifyWhenMaster(ctx, logger, "10.0.0.1"), time.Second).ShouldNot(
						Receive(),
					)
				})
			})
		})
	})
})
