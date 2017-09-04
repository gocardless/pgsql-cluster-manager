package pacemaker

import (
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/beevik/etree"
)

type CrmMon struct {
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

func NewCrmMon(timeout time.Duration) *CrmMon {
	return &CrmMon{systemExecutor{timeout}}
}

// Get returns a node from the crm_mon XML output, extracted using the given xpath. If we
// detect that pacemaker does not have quorum, then we error, as we should be able to rely
// on values being correct with respect to the quorate.
func (c CrmMon) Get(xpath string) (*etree.Element, error) {
	xmlOutput, err := c.CombinedOutput("crm_mon", "--as-xml")

	if err != nil {
		return nil, err
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(xmlOutput); err != nil {
		return nil, err
	}

	// We don't want to be returning values if we don't have quorum. Those values would only
	// ever be invalid to act upon.
	if doc.FindElement("crm_mon/summary/current_dc[@with_quorum='true']") == nil {
		return nil, errors.New("Cannot find designated controller with quorum")
	}

	return doc.FindElement(xpath), nil
}
