package main

import (
	"context"
	"debug/elf"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/kingpin"
	kitlog "github.com/go-kit/kit/log"
	"github.com/gocardless/pgsql-cluster-manager/pkg/acceptance"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	app         = kingpin.New("pgcm-acceptance", "Acceptance test suite for pgsql-cluster-manager").Version("0.0.0")
	workspace   = app.Flag("workspace", "Path to pgsql-cluster-manager workspace").Default(".").ExistingDir()
	dockerImage = app.Flag("docker-image", "Image to use for testing (postgres members").Default("gocardless/postgres-member").String()
)

func main() {
	if _, err := app.Parse(os.Args[1:]); err != nil {
		kingpin.Fatalf("%s, try --help", err)
	}

	if absWorkspace, err := filepath.Abs(*workspace); err != nil {
		kingpin.Fatalf("%v: failed to resolve --workspace, try --help", err)
	} else {
		*workspace = absWorkspace
	}

	binary := filepath.Join(*workspace, "bin/pgcm.linux_amd64")
	if _, err := elf.Open(binary); err != nil {
		kingpin.Fatalf("%s is not a valid linux binary: %v, try --help", binary, err)
	}

	RegisterFailHandler(Fail)

	SetDefaultEventuallyTimeout(time.Minute)
	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)

	RunSpecs(new(testing.T), "pgcm-acceptance")
}

var _ = Specify("Acceptance", func() {
	acceptance.RunAcceptance(
		context.Background(),
		kitlog.NewLogfmtLogger(os.Stderr),
		acceptance.ClusterOptions{
			Workspace:   *workspace,
			DockerImage: *dockerImage,
		},
	)
})
