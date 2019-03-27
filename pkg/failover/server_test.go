package failover

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/beevik/etree"
	kitlog "github.com/go-kit/kit/log"
	"github.com/gocardless/pgsql-cluster-manager/pkg/pacemaker"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func createElement(uname string) *etree.Element {
	return &etree.Element{
		Attr: []etree.Attr{
			etree.Attr{"", "uname", uname},
			etree.Attr{"", "id", "1"},
		},
	}
}

var _ = Describe("Server", func() {
	var (
		ctx     = context.Background()
		logger  kitlog.Logger
		server  *Server
		bouncer *fakePauser
		crm     *fakeCrm
		clock   *fakeClock
	)

	BeforeEach(func() {
		logger = kitlog.NewLogfmtLogger(GinkgoWriter)
		bouncer = new(fakePauser)
		crm = new(fakeCrm)
		clock = new(fakeClock)

		server = NewServer(logger, bouncer, crm)
		server.clock = clock
	})

	Describe("Pause", func() {
		subject := func(req *PauseRequest, pauseErr error) (*PauseResponse, error) {
			bouncer.On("Pause", mock.AnythingOfType("*context.timerCtx")).Return(pauseErr)
			clock.On("Now").Return(time.Now())

			return server.Pause(ctx, req)
		}

		Context("When PgBouncer pauses successfully", func() {
			It("Succeeds", func() {
				_, err := subject(&PauseRequest{Timeout: 5, Expiry: 0}, nil)
				Expect(err).NotTo(HaveOccurred())
			})

			Context("And expiry elapses", func() {
				// We would normally use the clock to defer a resume, but we want to simulate the
				// expiry being triggered, so we return zero to say "our time is up!" immediately.
				BeforeEach(func() {
					clock.On("Until", mock.AnythingOfType("time.Time")).Return(time.Duration(0))
				})

				Specify("We issue a resume", func() {
					resumed := make(chan interface{}, 1)
					bouncer.
						On("Resume", mock.AnythingOfType("*context.emptyCtx")).Return(nil).
						Run(func(args mock.Arguments) { resumed <- struct{}{} })

					// Set expiry to be a non-zero value, as otherwise we shortcut the expiry logic
					_, err := subject(&PauseRequest{Timeout: 5, Expiry: 1}, nil)

					Expect(err).NotTo(HaveOccurred())
					Eventually(resumed).Should(Receive(), "did not receive ack that resume occurred")
				})
			})
		})

		Context("When PgBouncer fails to pause", func() {
			It("Fails", func() {
				_, err := subject(&PauseRequest{Timeout: 5, Expiry: 0}, fmt.Errorf("nah"))
				Expect(err).To(HaveOccurred(), "expected Pause to return error")
			})
		})

		Context("When PgBouncer times out", func() {
			It("Returns explanatory error", func() {
				_, err := subject(&PauseRequest{Timeout: 0, Expiry: 0}, fmt.Errorf("context expired"))
				Expect(err).To(MatchError("rpc error: code = DeadlineExceeded desc = exceeded pause timeout"))
			})
		})
	})

	Describe("Migrate", func() {
		type lets struct {
			crmSyncElement      *etree.Element
			crmErr              error
			resolveAddressValue string
			resolveAddressErr   error
			migrateTo           string
			migrateErr          error
		}

		var (
			let lets
		)

		BeforeEach(func() {
			let = lets{}
		})

		subject := func() (*MigrateResponse, error) {
			clock.On("Now").Return(time.Now())

			crm.
				On("Get", ctx, []string{pacemaker.SyncXPath}).
				Return([]*etree.Element{let.crmSyncElement}, let.crmErr)

			crm.
				On("ResolveAddress", ctx, "1").
				Return(let.resolveAddressValue, let.resolveAddressErr)

			crm.
				On("Migrate", ctx, let.migrateTo).
				Return(let.migrateErr)

			return server.Migrate(ctx, &Empty{})
		}

		subjectErr := func() error {
			_, err := subject()
			return err
		}

		Context("When migration succeeds", func() {
			BeforeEach(func() {
				let.crmSyncElement = createElement("pg03")
				let.resolveAddressValue = "172.0.1.1"
				let.migrateTo = "pg03"
			})

			It("Succeeds", func() {
				Expect(subject()).To(
					PointTo(
						MatchFields(
							IgnoreExtras,
							Fields{
								"MigratingTo": Equal("pg03"),
								"Address":     Equal("172.0.1.1"),
							},
						),
					),
				)
			})
		})

		Context("When crm query fails", func() {
			BeforeEach(func() {
				let.crmErr = errors.New("oops")
			})

			It("Fails", func() {
				Expect(subjectErr()).To(
					MatchError("rpc error: code = Unknown desc = failed to query cib: oops"),
				)
			})
		})

		Context("When no sync node is found", func() {
			BeforeEach(func() {
				let.crmSyncElement = nil
			})

			It("Fails", func() {
				Expect(subjectErr()).To(
					MatchError("rpc error: code = NotFound desc = failed to find sync node"),
				)
			})
		})

		Context("When unable to resolve address", func() {
			BeforeEach(func() {
				let.crmSyncElement = createElement("pg03")
				let.resolveAddressErr = errors.New("corosync-cfgtool: not in $PATH")
			})

			It("Fails", func() {
				Expect(subjectErr()).To(
					MatchError("rpc error: code = Unknown desc = failed to resolve sync host IP address: corosync-cfgtool: not in $PATH"),
				)
			})
		})

		Context("When crm migration fails", func() {
			BeforeEach(func() {
				let.crmSyncElement = createElement("pg03")
				let.resolveAddressValue = "172.0.1.1"
				let.migrateTo = "pg03"
				let.migrateErr = errors.New("crm: not in $PATH")
			})

			It("Fails", func() {
				Expect(subjectErr()).To(
					MatchError("rpc error: code = Unknown desc = 'crm resource migrate pg03' failed: crm: not in $PATH"),
				)
			})
		})
	})
})
