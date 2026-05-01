package app

import "context"

// Closer defines a resource that can be closed gracefully.
type Closer interface {
	Close(context.Context) error
}
