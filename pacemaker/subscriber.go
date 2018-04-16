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
	pacemaker
	logger           *logrus.Logger
	handlers         map[string]handler
	nodes            []*crmNode
	nodeExpiry       time.Duration
	newTicker        func() *time.Ticker
	transform        func(context.Context, string) (string, error)
	pacemakerTimeout time.Duration
}

// crmNode represents an element in the pacemaker cib XML, selected using the given XPath,
// tracking change on the given Attribute.
type crmNode struct {
	Alias     string       // name of handler to call when node changes value
	XPath     string       // query into the CRM
	Attribute string       // attribute that determines the value passed to handler
	last      *cachedValue // stored value previously associated with this node
}

type cachedValue struct {
	seen  time.Time
	value string
}

type pacemaker interface {
	Get(context.Context, ...string) ([]*etree.Element, error)
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
// would construct a subscriber that will watch for changes in the cib to the 'name'
// attribute of the resource element with id 'master'. The subscriber will trigger the
// handler registered on 'master' with the new values of the attribute.
func WatchNode(alias, xpath, attribute string) func(*subscriber) {
	return func(s *subscriber) {
		s.nodes = append(s.nodes, &crmNode{
			alias, xpath, attribute, nil,
		})
	}
}

// WithLogger registers a logger that subscriber will use for output
func WithLogger(logger *logrus.Logger) func(*subscriber) {
	return func(s *subscriber) {
		s.logger = logger
	}
}

// WithTransform allows application of a general function to the values that are observed
// to change. This can be used, for example, to resolve IP addresses from node names when
// watching for changes in cluster roles.
func WithTransform(transform func(context.Context, string) (string, error)) func(*subscriber) {
	return func(s *subscriber) {
		s.transform = transform
	}
}

var defaultTransform = func(ctx context.Context, value string) (string, error) {
	return value, nil // by default, don't transform values
}

// NewSubscriber constructs a default subscriber configured to watch specific XML nodes
// inside the cib state.
func NewSubscriber(options ...func(*subscriber)) *subscriber {
	nullLogger := logrus.New()
	nullLogger.Out = ioutil.Discard

	s := &subscriber{
		pacemaker:        NewPacemaker(),
		logger:           nullLogger,             // for ease of use, default to using a null logger
		nodes:            []*crmNode{},           // start with an empty node list
		nodeExpiry:       30 * time.Second,       // expire nodes last value after this time
		handlers:         map[string]handler{},   // use AddHandler to add handlers
		pacemakerTimeout: 250 * time.Millisecond, // must be shorter than ticket interval
		transform:        defaultTransform,
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
		"xpath": node.XPath, "value": node.last.value,
		"handler": reflect.TypeOf(handler).String(),
	})

	contextLogger.Info("Observed node change, running handler")

	if err := handler.Run(node.Alias, node.last.value); err != nil {
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
				s.expireCache()
				s.logger.Debug("Polling pacemaker cib...")

				timeoutCtx, cancel := context.WithTimeout(ctx, s.pacemakerTimeout)

				if err := s.updateNodes(timeoutCtx, watchChan); err != nil {
					s.logger.WithError(err).Error("Failed to update nodes")
				}

				cancel()
			case <-ctx.Done():
				return
			}
		}
	}()

	return watchChan
}

// expireCache erases the last seen value for each node, ensuring that the next time we
// scrape pacemaker, we'll push the value to our handlers. This prevents etcd from losing
// the value and never having it re-populated, as we think it's never been changed.
func (s *subscriber) expireCache() {
	for _, node := range s.nodes {
		if node.last != nil && time.Since(node.last.seen) > s.nodeExpiry {
			s.logger.WithField("node", node.Alias).Info("Expiring cached value for node...")
			node.last = nil
		}
	}
}

// updateNodes queries crm to find current node values, updates the value on each node and
// sends nodes that have been updated down the given channel.
func (s *subscriber) updateNodes(ctx context.Context, updated chan *crmNode) error {
	elements, err := s.pacemaker.Get(ctx, s.xpaths()...)

	if err != nil {
		return err
	}

	for idx, node := range s.nodes {
		if elements[idx] == nil {
			break // the node may not be in the cib
		}

		value := elements[idx].SelectAttrValue(node.Attribute, "")
		value, err := s.transform(ctx, value)

		if err != nil {
			return err
		}

		if node.last == nil || node.last.value != value {
			node.last = &cachedValue{
				value: value,
				seen:  time.Now(),
			}

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
