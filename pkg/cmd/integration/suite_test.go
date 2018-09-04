package integration

import (
	"testing"

	"github.com/coreos/etcd/clientv3"
	"github.com/gocardless/pgsql-cluster-manager/pkg/etcd/integration"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	client *clientv3.Client
)

func TestSuite(t *testing.T) {
	RegisterFailHandler(Fail)

	// Boot an etcd cluster for use across this test suite. RandomKey() can be used to
	// generate unique keys to prevent tests interacting with each other.
	var cleanup func()
	var err error

	client, cleanup, err = integration.StartEtcd()
	Expect(err).NotTo(HaveOccurred())
	defer cleanup()

	RunSpecs(t, "pkg/cmd/integration")
}
