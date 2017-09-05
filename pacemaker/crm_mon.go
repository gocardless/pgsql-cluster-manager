package pacemaker

import (
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/beevik/etree"
)

// CrmMon wraps the crm_mon executable provided by pacemaker, that queries the cib and
// outputs information on node roles. crm_mon was chosen over cibadmin (which provides
// direct querying of the cib) because the output from cibadmin only indirectly specifies
// which resources are present in which node.
//
// Trying to detect a Postgres primary from the cibadmin output could only be achieved by
// searching for the node where Postgresql-data-status was LATEST, for example, which is
// much less direct than using crm_mon's output where the location of the PostgresqlVIP is
// clear.
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

// Get returns nodes from the crm_mon XML output, extracted using the given XPaths. If we
// detect that pacemaker does not have quorum, then we error, as we should be able to rely
// on values being correct with respect to the quorate.
func (c CrmMon) Get(xpaths ...string) ([]*etree.Element, error) {
	nodes := make([]*etree.Element, 0)
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

	for _, xpath := range xpaths {
		nodes = append(nodes, doc.FindElement(xpath))
	}

	return nodes, nil
}
