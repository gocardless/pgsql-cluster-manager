package subscriber

import (
	"golang.org/x/net/context"
)

type Subscriber interface {
	Start(context.Context, map[string]Handler) error
}

// Handler is the minimal interface that should respond to Subscriber events
type Handler interface {
	Run(string, string) error
}
