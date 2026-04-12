package lifecycle

import "context"

type Closer interface {
	Close(context.Context) error
}
