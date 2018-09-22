package pacemaker

import (
	"io/ioutil"
	"time"

	"golang.org/x/net/context"

	"github.com/coreos/etcd/mvcc/mvccpb"
	kitlog "github.com/go-kit/kit/log"
	"github.com/onsi/gomega/types"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("NewStream", func() {
	var (
		ctx      context.Context
		cancel   func()
		crm      *Pacemaker
		executor *fakeExecutor
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), time.Second)
		executor = new(fakeExecutor)
		crm = &Pacemaker{executor: executor}
	})

	AfterEach(func() {
		cancel()
	})

	loadFixtures := func(fixtures ...string) {
		for _, fixture := range fixtures {
			content, err := ioutil.ReadFile(fixture)
			Expect(err).NotTo(HaveOccurred())

			executor.
				On("CombinedOutput",
					mock.Anything, "cibadmin", []string{"--query", "--local"},
				).
				Return(content, nil).
				Once()
		}

		executor.
			On("CombinedOutput",
				mock.Anything, "cibadmin", []string{"--query", "--local"},
			).
			Return([]byte(""), nil)
	}

	newStream := func() <-chan *mvccpb.KeyValue {
		out, _ := NewStream(
			kitlog.NewNopLogger(),
			crm,
			StreamOptions{
				Ctx:       ctx,
				Attribute: "uname",
				XPaths: []AliasedXPath{
					AliasedXPath{Alias("master"), XPath(MasterXPath)},
					AliasedXPath{Alias("sync"), XPath(SyncXPath)},
				},
				PollInterval: time.Millisecond,
				GetTimeout:   0,
			},
		)

		return out
	}

	matchKv := func(key, value string) types.GomegaMatcher {
		return PointTo(
			MatchFields(
				IgnoreExtras,
				Fields{
					"Key":   Equal([]byte(key)),
					"Value": Equal([]byte(value)),
				},
			),
		)
	}

	Context("With changing pacemaker state", func() {
		BeforeEach(func() {
			loadFixtures(
				"./testdata/cib_sync_async_master.xml",
				"./testdata/cib_async_master_sync.xml",
				"./testdata/cib_async_sync_master.xml",
			)
		})

		It("Pushes state as mvccpb.KeyValues down channel", func() {
			out := newStream()

			Eventually(out).Should(Receive(matchKv("master", "pg03")))
			Eventually(out).Should(Receive(matchKv("sync", "pg01")))

			Eventually(out).Should(Receive(matchKv("master", "pg02")))
			Eventually(out).Should(Receive(matchKv("sync", "pg03")))

			Eventually(out).Should(Receive(matchKv("master", "pg03")))
			Eventually(out).Should(Receive(matchKv("sync", "pg02")))

			cancel()
		})
	})
})
