package pacemaker

import (
	"strings"
	"time"

	"github.com/coreos/etcd/mvcc/mvccpb"
	kitlog "github.com/go-kit/kit/log"
	"golang.org/x/net/context"
)

type StreamOptions struct {
	Ctx          context.Context
	Attribute    string
	XPaths       []AliasedXPath
	PollInterval time.Duration
	GetTimeout   time.Duration
}

type AliasedXPath struct {
	Alias
	XPath
}

type Alias string
type XPath string

func AliasXPath(alias, xpath string) AliasedXPath {
	return AliasedXPath{Alias(alias), XPath(xpath)}
}

func NewStream(logger kitlog.Logger, crm *Pacemaker, opt StreamOptions) (<-chan *mvccpb.KeyValue, <-chan struct{}) {
	out, done := make(chan *mvccpb.KeyValue), make(chan struct{})

	// Precompute the xpaths in a form that is easily supplied to crm.Get()
	xpaths := make([]string, 0)
	for _, ax := range opt.XPaths {
		xpaths = append(xpaths, string(ax.XPath))
	}

	logger = kitlog.With(logger, "xpaths", strings.Join(xpaths, ","))

	go func() {
		for {
			select {
			case <-time.After(opt.PollInterval):
				logger.Log("event", "poll.start")
				getCtx, cancel := context.WithTimeout(opt.Ctx, opt.GetTimeout)
				nodes, err := crm.Get(getCtx, xpaths...)
				cancel()

				if err != nil {
					logger.Log("event", "poll.error", "error", err)
					continue
				}

				for idx, ax := range opt.XPaths {
					if nodes[idx] == nil {
						continue
					}

					out <- &mvccpb.KeyValue{
						Key:   []byte(ax.Alias),
						Value: []byte(nodes[idx].SelectAttrValue(opt.Attribute, "")),
					}
				}
			case <-opt.Ctx.Done():
				logger.Log("event", "poll.stop", "msg", "context expired, stopping")
				close(out)
				close(done)

				return
			}
		}
	}()

	return out, done
}
