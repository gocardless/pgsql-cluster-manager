package subscriber

import (
	"errors"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/beevik/etree"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type fakeCrmMon struct{ mock.Mock }

func (c fakeCrmMon) Get(xpaths ...string) ([]*etree.Element, error) {
	args := c.Called(xpaths)
	return args.Get(0).([]*etree.Element), args.Error(1)
}

func fakeTicker(ctx context.Context) (*time.Ticker, func()) {
	tickChan := make(chan time.Time)
	ticker := time.Ticker{C: tickChan}

	return &ticker, func() {
		// Non-blocking send, so we can repeatedly tick until the end of time
		select {
		case tickChan <- time.Now():
		default:
		}
	}
}

func TestStart(t *testing.T) {
	makeResources := func(values ...string) []*etree.Element {
		elements := make([]*etree.Element, len(values))

		for idx, value := range values {
			elements[idx] = etree.NewElement("resource")
			elements[idx].CreateAttr("name", value)
		}

		return elements
	}

	testCases := []struct {
		name       string
		nodes      []*CrmNode
		getParams  []string
		getResults [][]*etree.Element
		handlers   map[string]Handler
	}{
		{
			"when node changes, handler is called",
			[]*CrmNode{
				&CrmNode{
					Alias:     "/master",
					XPath:     "/resource[@id='PostgresqlVIP']",
					Attribute: "name",
				},
			},
			[]string{"/resource[@id='PostgresqlVIP']"},
			[][]*etree.Element{
				makeResources("larry"),
				makeResources("moe"),
			},
			func() map[string]Handler {
				handler := new(FakeHandler)

				handler.On("Run", "/master", "larry").Return(nil).Once()
				handler.On("Run", "/master", "moe").Return(nil).Once()

				return map[string]Handler{"/master": handler}
			}(),
		},
		{
			"when nodes don't change between polling, handler is only called once",
			[]*CrmNode{
				&CrmNode{
					Alias:     "/master",
					XPath:     "/resource[@id='PostgresqlVIP']",
					Attribute: "name",
				},
			},
			[]string{"/resource[@id='PostgresqlVIP']"},
			[][]*etree.Element{
				makeResources("larry"),
				makeResources("larry"),
				makeResources("larry"),
			},
			func() map[string]Handler {
				handler := new(FakeHandler)
				handler.On("Run", "/master", "larry").Return(nil).Once()

				return map[string]Handler{"/master": handler}
			}(),
		},
		{
			"when watching multiple nodes, we call the right handlers",
			[]*CrmNode{
				&CrmNode{
					Alias:     "/master",
					XPath:     "/resource[@id='PostgresqlVIP']",
					Attribute: "name",
				},
				&CrmNode{
					Alias:     "/pgbouncer",
					XPath:     "/resource[@id='PgBouncerVIP']",
					Attribute: "name",
				},
			},
			[]string{"/resource[@id='PostgresqlVIP']", "/resource[@id='PgBouncerVIP']"},
			[][]*etree.Element{
				makeResources("larry", "curly"),
				makeResources("larry", "moe"),
				makeResources("curly", "moe"),
			},
			func() map[string]Handler {
				masterHandler := new(FakeHandler)
				masterHandler.On("Run", "/master", "larry").Return(nil).Once()
				masterHandler.On("Run", "/master", "curly").Return(nil).Once()

				bouncerHandler := new(FakeHandler)
				bouncerHandler.On("Run", "/pgbouncer", "curly").Return(nil).Once()
				bouncerHandler.On("Run", "/pgbouncer", "moe").Return(nil).Once()

				return map[string]Handler{"/master": masterHandler, "/pgbouncer": bouncerHandler}
			}(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Stub crmMon to expect the given tc.getParams, returning tc.getResults. The last
			// stub will be an error, which will cause Start to finish executing.
			crmMon := new(fakeCrmMon)
			for _, results := range tc.getResults {
				crmMon.On("Get", tc.getParams).Return(results, nil).Once()
			}
			crmMon.On("Get", tc.getParams).Return([]*etree.Element{}, errors.New("out of stubs"))

			ticker, tick := fakeTicker(ctx)
			done := make(chan error, 1)

			// Start the subscriber, which is controlled by our fake ticker
			go func() {
				done <- NewCrm(crmMon, func() *time.Ticker { return ticker }, tc.nodes).Start(
					ctx, tc.handlers,
				)
			}()

			// Wait for the subscriber to conclude, or for us to timeout
			require.Nil(t, func() error {
				timeout := time.After(time.Second)

				for {
					select {
					case <-done:
						return nil
					case <-timeout:
						return errors.New("timed out")
					default:
						tick()
					}
				}
			}())

			// Verify all our handlers have received the calls we expected them to
			for _, handler := range tc.handlers {
				fakeHandler, ok := handler.(*FakeHandler)
				require.True(t, ok)

				fakeHandler.AssertExpectations(t)
			}
		})
	}
}
