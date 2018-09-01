package integration

import (
	"context"
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/gocardless/pgsql-cluster-manager/pkg/pgbouncer"
	"github.com/jackc/pgx"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func tryEnviron(key, otherwise string) string {
	if value, found := os.LookupEnv(key); found {
		return value
	}

	return otherwise
}

var _ = Describe("PgBouncer", func() {
	var (
		ctx     context.Context
		cancel  func()
		bouncer *pgbouncer.PgBouncer
		cleanup func()
		err     error

		// We expect a Postgres database to be running for integration tests, and that
		// environment variables are appropriately configured to permit access.
		database = tryEnviron("PGDATABASE", "postgres")
		host     = tryEnviron("PGHOST", "127.0.0.1")
		user     = tryEnviron("PGUSER", "postgres")
		port     = tryEnviron("PGPORT", "5432")
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		bouncer, cleanup, err = StartPgBouncer(database, user, port)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cleanup()
	})

	readlogs := func() string {
		workspace := path.Dir(bouncer.ConfigFile)
		logs, err := ioutil.ReadFile(path.Join(workspace, "pgbouncer.log"))

		Expect(err).NotTo(HaveOccurred())
		return string(logs)
	}

	Describe("ShowDatabases", func() {
		It("Correctly parses databases", func() {
			databases, err := bouncer.ShowDatabases(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(databases).To(ConsistOf(
				pgbouncer.Database{
					Name: "pgbouncer",
					Port: "6432",
				},
				pgbouncer.Database{
					Name: database,
					Port: port,
					Host: "{{.Host}}", // this is the initial config value
				},
			))
		})
	})

	Describe("Reload", func() {
		It("Succeeds, triggering config reload", func() {
			Expect(bouncer.ShowDatabases(ctx)).To(ContainElement(
				MatchFields(IgnoreExtras, Fields{
					"Name": Equal(database),
					"Host": Equal("{{.Host}}"),
				}),
			))

			Expect(bouncer.GenerateConfig("new-host")).To(Succeed())
			Expect(bouncer.Reload(ctx)).To(Succeed())
			Eventually(readlogs).Should(ContainSubstring("LOG RELOAD command issued"))

			Expect(bouncer.ShowDatabases(ctx)).To(ContainElement(
				MatchFields(IgnoreExtras, Fields{
					"Name": Equal(database),
					"Host": Equal("new-host"),
				}),
			))
		})
	})

	Describe("Pause", func() {
		Context("When not already paused", func() {
			It("Succeeds", func() {
				Expect(bouncer.Pause(ctx)).To(Succeed())
				Eventually(readlogs).Should(ContainSubstring("LOG PAUSE command issued"))
			})
		})

		Context("When session is blocking pause", func() {
			connectToDatabase := func() *pgx.Conn {
				executor := bouncer.Executor.(pgbouncer.AuthorizedExecutor)
				conn, err := pgx.Connect(pgx.ConnConfig{
					Host:     executor.SocketDir,
					Port:     6432,
					Database: database,
					User:     user,
				})

				Expect(err).NotTo(HaveOccurred())
				return conn
			}

			It("Times out and resumes", func() {
				// Point the PgBouncer configuration at our integration Postgres database
				Expect(bouncer.GenerateConfig(host)).To(Succeed())
				Expect(bouncer.Reload(ctx)).To(Succeed())

				conn := connectToDatabase()
				defer conn.Close()

				// Establish a session pooled connection, which should prevent PgBouncer from
				// being able to pause the pool.
				Expect(conn.ExecEx(ctx, "select now()", nil)).NotTo(BeNil())

				timeoutCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
				defer cancel()

				// We can expect this pause command not to succeed, as it will timeout
				Expect(bouncer.Pause(timeoutCtx)).To(MatchError("context deadline exceeded"))

				// New commands that come to the database
				anotherConn := connectToDatabase()
				defer anotherConn.Close()

				Expect(conn.ExecEx(ctx, "select now()", nil)).NotTo(BeNil())
			})
		})

		Context("When already paused", func() {
			It("Succeeds", func() {
				Expect(bouncer.Pause(ctx)).To(Succeed())
				Expect(bouncer.Pause(ctx)).To(Succeed())

				Eventually(readlogs).Should(ContainSubstring("LOG PAUSE command issued"))
				Eventually(readlogs).Should(ContainSubstring("ERROR already suspended/paused"))
			})
		})
	})

	Describe("Resume", func() {
		Context("When paused", func() {
			BeforeEach(func() { Expect(bouncer.Pause(ctx)).To(Succeed()) })

			It("Succeeds", func() {
				Expect(bouncer.Resume(ctx)).To(Succeed())
				Eventually(readlogs).Should(ContainSubstring("LOG RESUME command issued"))
			})
		})

		Context("When not paused", func() {
			It("Succeeds", func() {
				Expect(bouncer.Resume(ctx)).To(Succeed())
				Eventually(readlogs).Should(ContainSubstring("ERROR pooler is not paused/suspended"))
			})
		})
	})
})
