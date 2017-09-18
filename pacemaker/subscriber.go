package pacemaker

import (
	"io/ioutil"
	"reflect"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/beevik/etree"
	"golang.org/x/net/context"
)

type subscriber struct {
	crmStore
	logger       *logrus.Logger
	handlers     map[string]handler
	nodes        []*crmNode
	newTicker    func() *time.Ticker
	errorHandler func(error)
}

// crmNode represents an element in the output XML of crm_mon, selected using the given
// XPath, tracking change on the given Attribute.
type crmNode struct {
	Alias     string // name of handler to call when node changes value
	XPath     string // query into the CRM
	Attribute string // attribute that determines the value passed to handler
	value     string // stored value previously associated with this node
}

type crmStore interface {
	Get(...string) ([]*etree.Element, error)
}

type handler interface {
	Run(string, string) error
}

// WatchNode creates an option for subscribers that activates watching of a specific
// attribute of the element found with the given xpath.
//
// pacemaker.NewSubscriber(
//   pacemaker.WatchNode("master", "//resource[@id='master']", "name"),
// )
//
// would construct a subscriber that will watch for changes in crm_mon to the 'name'
// attribute of the resource element with id 'master'. The subscriber will trigger the
// handler registered on 'master' with the new values of the attribute.
func WatchNode(alias, xpath, attribute string) func(*subscriber) {
	return func(s *subscriber) {
		s.nodes = append(s.nodes, &crmNode{
			alias, xpath, attribute, "",
		})
	}
}

// CrmErrorHandler creates an option for subscribers which will configure a handler for
// crm errors. Whenever a crm operation returns an error, this handler will be called.
func CrmErrorHandler(errorHandler func(error)) func(*subscriber) {
	return func(s *subscriber) {
		s.errorHandler = errorHandler
	}
}

// WithLogger registers a logger that subscriber will use for output
func WithLogger(logger *logrus.Logger) func(*subscriber) {
	return func(s *subscriber) {
		s.logger = logger
	}
}

// NewSubscriber constructs a default subscriber configured to watch specific XML nodes
// inside the crm_mon state.
func NewSubscriber(options ...func(*subscriber)) *subscriber {
	nullLogger := logrus.New()
	nullLogger.Out = ioutil.Discard

	s := &subscriber{
		crmStore: NewCrmMon(250 * time.Millisecond), // required to be less than the ticker
		logger:   nullLogger,                        // for ease of use, default to using a null logger
		nodes:    []*crmNode{},                      // start with an empty node list
		handlers: map[string]handler{},              // use AddHandler to add handlers
		newTicker: func() *time.Ticker {
			return time.NewTicker(500 * time.Millisecond) // 500ms provides frequent updates
		},
	}

	for _, option := range options {
		option(s)
	}

	return s
}

// AddHandler registers a new handler to be run on changes to nodes with the given alias
func (s *subscriber) AddHandler(alias string, h handler) *subscriber {
	s.logger.WithFields(map[string]interface{}{
		"alias": alias, "handler": reflect.TypeOf(h).String(),
	}).Info("Registering handler")

	s.handlers[alias] = h
	return s
}

// Start will begin listening for changes to the configured node list. Whenever any
// changes are detected to a crmNode element attribute, the appropriate handler will be
// selected using the crmNode 'Alias' and called with the value of the element attribute.
func (s *subscriber) Start(ctx context.Context) {
	s.logger.Info("Starting pacemaker subscriber")

	for updatedNode := range s.watch(ctx) {
		s.processUpdatedNode(updatedNode)
	}

	s.logger.Info("Finishing pacemaker handler, watcher closed")
}

func (s *subscriber) processUpdatedNode(node *crmNode) {
	handler := s.handlers[node.Alias]

	contextLogger := s.logger.WithFields(map[string]interface{}{
		"alias": node.Alias, "attribute": node.Attribute,
		"xpath": node.XPath, "value": node.value,
		"handler": reflect.TypeOf(handler).String(),
	})

	contextLogger.Info("Observed node change, running handler")

	if err := handler.Run(node.Alias, node.value); err != nil {
		contextLogger.WithError(err).Error("Failed to run handler")
	}
}

func (s *subscriber) watch(ctx context.Context) chan *crmNode {
	ticker := s.newTicker()
	watchChan := make(chan *crmNode)

	go func() {
		defer ticker.Stop()
		defer close(watchChan)

		for {
			select {
			case <-ticker.C:
				if err := s.updateNodes(watchChan); err != nil {
					if s.errorHandler != nil {
						s.errorHandler(err)
					}
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
func (s *subscriber) updateNodes(updated chan *crmNode) error {
	elements, err := s.crmStore.Get(s.xpaths()...)

	if err != nil {
		return err
	}

	for idx, node := range s.nodes {
		if elements[idx] == nil {
			break // the node may not be in the cib
		}

		value := elements[idx].SelectAttrValue(node.Attribute, "")

		if node.value != value {
			node.value = value
			updated <- node
		}
	}

	return nil
}

func (s *subscriber) xpaths() []string {
	xpaths := make([]string, len(s.nodes))

	for idx, node := range s.nodes {
		xpaths[idx] = node.XPath
	}

	return xpaths
}
