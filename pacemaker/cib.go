package pacemaker

import (
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/beevik/etree"
)

var (
	MasterXPath = "//node/instance_attributes/nvpair[@value='LATEST']/../.."
	SyncXPath   = "//node/instance_attributes/nvpair[@value='STREAMING|SYNC']/../.."
	AsyncXPath  = "//node/instance_attributes/nvpair[@value='STREAMING|POTENTIAL']/../.."
)

// Cib wraps the cibadmin executable provided by pacemaker, that queries the cib and
// outputs information on node roles.
type Cib struct {
	executor
}

type executor interface {
	CombinedOutput(string, ...string) ([]byte, error)
}

type systemExecutor struct {
	timeout time.Duration
}

func (e systemExecutor) CombinedOutput(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func WithExecutor(e executor) func(*Cib) {
	return func(c *Cib) {
		c.executor = e
	}
}

func NewCib(options ...func(*Cib)) *Cib {
	c := &Cib{systemExecutor{500 * time.Millisecond}}

	for _, option := range options {
		option(c)
	}

	return c
}

// Get returns nodes from the cibadmin XML output, extracted using the given XPaths. If we
// detect that pacemaker does not have quorum, then we error, as we should be able to rely
// on values being correct with respect to the quorate.
func (c Cib) Get(xpaths ...string) ([]*etree.Element, error) {
	nodes := make([]*etree.Element, 0)
	xmlOutput, err := c.CombinedOutput("cibadmin", "--query", "--local")

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
		return nil, errors.New("no quorum")
	}

	for _, xpath := range xpaths {
		nodes = append(nodes, doc.FindElement(xpath))
	}

	return nodes, nil
}
