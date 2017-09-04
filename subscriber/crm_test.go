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
	makeElement := func(tag, attr, value string) []*etree.Element {
		element := etree.NewElement("resource")
		element.CreateAttr(attr, value)

		return []*etree.Element{element}
	}

	t.Run("when node changes, calls handler", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ticker, tick := fakeTicker(ctx)

		crmMon := new(fakeCrmMon)
		crmMonGet := func() *mock.Call { return crmMon.On("Get", []string{"/resource[@id='PostgresqlVIP']"}) }

		crmMonGet().Return(makeElement("resource", "name", "larry"), nil).Times(2)
		crmMonGet().Return(makeElement("resource", "name", "moe"), nil).Times(2)
		crmMonGet().Return(makeElement("resource", "name", "curly"), nil).Once()

		crmMonGet().Return([]*etree.Element{}, errors.New("out of stubs"))

		crm := NewCrm(
			crmMon,
			func() *time.Ticker { return ticker },
			[]*CrmNode{
				&CrmNode{
					Alias:     "/master",
					XPath:     "/resource[@id='PostgresqlVIP']",
					Attribute: "name",
				},
			},
		)

		handler := new(FakeHandler)

		handler.On("Run", "/master", "larry").Return(nil).Once()
		handler.On("Run", "/master", "moe").Return(nil).Once()
		handler.On("Run", "/master", "curly").Return(nil).Once()

		done := make(chan error, 1)

		go func() {
			done <- crm.Start(ctx, map[string]Handler{
				"/master": handler,
			})
		}()

		// Wait for the subscriber to conclude, or for us to time out
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

		handler.AssertExpectations(t)
	})
}
