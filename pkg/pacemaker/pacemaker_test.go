package pacemaker

import (
	"fmt"
	"io/ioutil"

	"golang.org/x/net/context"

	"github.com/beevik/etree"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Pacemaker", func() {
	var (
		ctx      = context.Background()
		crm      *Pacemaker
		executor *fakeExecutor
	)

	BeforeEach(func() {
		executor = new(fakeExecutor)
		crm = &Pacemaker{executor: executor}
	})

	Describe("Get", func() {
		loadFixture := func(fixture string, mockErr error) {
			content, err := ioutil.ReadFile(fixture)
			Expect(err).NotTo(HaveOccurred())

			executor.
				On("CombinedOutput",
					ctx, "cibadmin", []string{"--query", "--local"},
				).
				Return(content, mockErr)
		}

		get := func(xpath, attr string) string {
			nodes, err := crm.Get(ctx, xpath)

			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(1))

			return nodes[0].SelectAttrValue(attr, "")
		}

		Context("With full cluster", func() {
			BeforeEach(func() {
				loadFixture("./testdata/cib_sync_async_master.xml", nil)
			})

			It("Finds master", func() {
				Expect(get(MasterXPath, "uname")).To(Equal("pg03"))
			})

			It("Finds sync", func() {
				Expect(get(SyncXPath, "uname")).To(Equal("pg01"))
			})

			It("Finds async", func() {
				Expect(get(AsyncXPath, "uname")).To(Equal("pg02"))
			})
		})

		Context("With no quorum", func() {
			BeforeEach(func() {
				loadFixture("./testdata/cib_master_died_died.xml", nil)
			})

			It("Returns error", func() {
				_, err := crm.Get(ctx, MasterXPath)
				Expect(err).To(MatchError(NoQuorumError{}))
			})
		})

		Context("With missing async", func() {
			BeforeEach(func() {
				loadFixture("./testdata/cib_master_sync_died.xml", nil)
			})

			It("Returns nil for async", func() {
				Expect(crm.Get(ctx, AsyncXPath)).To(Equal([]*etree.Element{nil}))
			})
		})
	})

	Describe("ResolveAddress", func() {
		loadFixture := func(nodeID, fixture string, err error) {
			executor.
				On("CombinedOutput",
					ctx, "corosync-cfgtool", []string{"-a", nodeID},
				).
				Return([]byte(fixture), err)
		}

		Context("When corosync-cfgtool works", func() {
			BeforeEach(func() {
				loadFixture("1", "172.17.0.4\n", nil)
			})

			It("Identifies node IP address", func() {
				Expect(crm.ResolveAddress(ctx, "1")).To(Equal("172.17.0.4"))
			})
		})

		Context("When nodeID is invalid", func() {
			It("Returns error", func() {
				_, err := crm.ResolveAddress(ctx, "invalid-node-id")
				Expect(err).To(MatchError(InvalidNodeIDError("invalid-node-id")))
			})
		})

		Context("When corosync-cfgtool returns error", func() {
			BeforeEach(func() {
				loadFixture("1", "", fmt.Errorf("exec: \"corosync-cfgtool\": executable file not found in $PATH"))
			})

			It("Returns error", func() {
				_, err := crm.ResolveAddress(ctx, "1")
				Expect(err).To(HaveOccurred())
			})
		})
	})
})

type fakeExecutor struct{ mock.Mock }

func (e fakeExecutor) CombinedOutput(ctx context.Context, name string, arg ...string) ([]byte, error) {
	args := e.Called(ctx, name, arg)
	return args.Get(0).([]byte), args.Error(1)
}
