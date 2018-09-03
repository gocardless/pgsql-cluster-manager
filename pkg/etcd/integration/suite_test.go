package integration

import (
	"testing"

	"github.com/coreos/etcd/clientv3"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	client  *clientv3.Client
	cleanup func()
	etcdErr error
)

func TestSuite(t *testing.T) {
	RegisterFailHandler(Fail)

	// All tests in this suite require access to an etcd cluster. Boot one that we can use
	// for everything, and rely on RandomKey() to generate unique keys.
	client, cleanup, etcdErr = StartEtcd()
	Expect(etcdErr).NotTo(HaveOccurred())
	defer cleanup()

	RunSpecs(t, "pkg/cmd/integration")
}
