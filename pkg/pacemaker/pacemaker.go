package pacemaker

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/net/context"

	"github.com/beevik/etree"
)

var (
	MasterXPath = "//node/instance_attributes/nvpair[@value='LATEST']/../.."
	SyncXPath   = "//node/instance_attributes/nvpair[@value='STREAMING|SYNC']/../.."
	AsyncXPath  = "//node/instance_attributes/nvpair[@value='STREAMING|POTENTIAL']/../.."
)

// Pacemaker wraps the executables provided by pacemaker, providing querying of the cib as
// well as running commands against crm.
type Pacemaker struct {
	executor
}

type executor interface {
	CombinedOutput(context.Context, string, ...string) ([]byte, error)
}

type systemExecutor struct{}

func (e systemExecutor) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func NewPacemaker(exec executor) *Pacemaker {
	if exec == nil {
		exec = systemExecutor{}
	}

	return &Pacemaker{exec}
}

type NoQuorumError struct{}

func (e NoQuorumError) Error() string {
	return "pacemaker reports no quorum, cannot get results"
}

// Get returns nodes from the cibadmin XML output, extracted using the given XPaths. If we
// detect that pacemaker does not have quorum, then we error, as we should be able to rely
// on values being correct with respect to the quorate.
func (p Pacemaker) Get(ctx context.Context, xpaths ...string) ([]*etree.Element, error) {
	nodes := make([]*etree.Element, 0)
	xmlOutput, err := p.CombinedOutput(ctx, "cibadmin", "--query", "--local")

	if err != nil {
		return nil, err
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(xmlOutput); err != nil {
		return nil, err
	}

	// We don't want to be returning values if we don't have quorum. Those values would only
	// ever be invalid to act upon.
	if doc.FindElement("cib[@have-quorum='1']") == nil {
		return nil, NoQuorumError{}
	}

	for _, xpath := range xpaths {
		nodes = append(nodes, doc.FindElement(xpath))
	}

	return nodes, nil
}

type InvalidNodeIDError string

func (e InvalidNodeIDError) Error() string {
	return fmt.Sprintf("invalid nodeID, must be single integer: '%s'", string(e))
}

// ResolveAddress will find the IP address for a given node ID. Node IDs are numeric, but
// we'll typically extract them from XML that will yield strings. Given we'll be passing
// them as string executable arguments it makes sense to keep everything homomorphic.
func (p Pacemaker) ResolveAddress(ctx context.Context, nodeID string) (string, error) {
	if !regexp.MustCompile("^\\s*(\\d+)$").MatchString(nodeID) {
		return "", InvalidNodeIDError(nodeID)
	}

	output, err := p.CombinedOutput(ctx, "corosync-cfgtool", "-a", nodeID)

	if err != nil {
		return "", errors.Wrap(err, "failed to run corosync-cfgtool")
	}

	return strings.TrimSpace(string(output)), nil
}

// Migrate will issue a resource migration of msPostgresql to the given node
func (p Pacemaker) Migrate(ctx context.Context, to string) error {
	_, err := p.CombinedOutput(ctx, "crm", "resource", "migrate", "msPostgresql", to)

	if err != nil {
		return errors.Wrap(err, "failed to execute crm migration")
	}

	return err
}

// Unmigrate will remove constraints previously created by migrate
func (p Pacemaker) Unmigrate(ctx context.Context) error {
	_, err := p.CombinedOutput(ctx, "crm", "resource", "unmigrate", "msPostgresql")

	if err != nil {
		return errors.Wrap(err, "failed to execute crm resource unmigrate")
	}

	return err
}
