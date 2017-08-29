package subscriber

import "golang.org/x/net/context"

type Subscriber interface {
	RegisterHandler(string, Handler)
	Start(context.Context) error
	Shutdown() error
}
