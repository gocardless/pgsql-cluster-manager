package subscriber

import (
	"time"

	"github.com/beevik/etree"
	"golang.org/x/net/context"
)

type crm struct {
	crmStore
	nodes     []*CrmNode
	newTicker func() *time.Ticker
}

type CrmNode struct {
	Alias     string // name of handler to call when node changes value
	XPath     string // query into the CRM
	Attribute string // attribute that determines the value passed to handler
	value     string // stored value previously associated with this node
}

type crmStore interface {
	Get(...string) ([]*etree.Element, error)
}

func NewCrm(store crmStore, newTicker func() *time.Ticker, nodes []*CrmNode) Subscriber {
	return &crm{store, nodes, newTicker}
}

func (s crm) Start(ctx context.Context, handlers map[string]Handler) error {
	for updatedNode := range s.watch(ctx) {
		if handler := handlers[updatedNode.Alias]; handler != nil {
			handler.Run(updatedNode.Alias, updatedNode.value)
		}
	}

	return nil
}

func (s crm) watch(ctx context.Context) chan *CrmNode {
	ticker := s.newTicker()
	watchChan := make(chan *CrmNode, len(s.nodes))

	go func() {
		defer ticker.Stop()
		defer close(watchChan)

		for {
			select {
			case <-ticker.C:
				if err := s.updateNodes(watchChan); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return watchChan
}

// updateNodes queries crm to find current node values, updates the value on each node and
// sends nodes that have been updated down the given channel.
func (s crm) updateNodes(updated chan *CrmNode) error {
	elements, err := s.crmStore.Get(s.xpaths()...)

	if err != nil {
		return err
	}

	for idx, node := range s.nodes {
		value := elements[idx].SelectAttrValue(node.Attribute, "")

		if node.value != value {
			node.value = value
			updated <- node
		}
	}

	return nil
}

func (s crm) xpaths() []string {
	xpaths := make([]string, len(s.nodes))

	for idx, node := range s.nodes {
		xpaths[idx] = node.XPath
	}

	return xpaths
}
