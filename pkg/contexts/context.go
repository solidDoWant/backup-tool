package contexts

import (
	"context"
	"time"
)

// This should only contain fields that are realistically needed collectively
// whenever one is needed, while processing a DR command. This reduces the
// number of arguments that need to be passed around in the codebase. Doing so
// improves readability, at the cost of more "global" type values. However,
// given that nearly everything will need these values anyway, there isn't much
// downside in grouping them together.
type Context struct {
	context.Context
	// *slog.Logger // TODO charm.Log
}

func NewContext(ctx context.Context) *Context {
	return &Context{
		Context: ctx,
		// Logger:  logger,
	}
}

// Important: this is a shallow copy, so changes to nested values will be seen in the original context.
// However, changing the field values themselves will not be seen in the original context.
func (c *Context) ShallowCopy() *Context {
	// Go is copy by value, so passing the dereferenced value makes a shallow
	// copy of it.
	return func(c Context) *Context {
		return &c
	}(*c)
}

func WithTimeout(parent *Context, timeout time.Duration) (*Context, context.CancelFunc) {
	var ctx context.Context
	var cancel context.CancelFunc
	if timeout == 0 {
		ctx, cancel = context.WithCancel(parent)
	} else {
		ctx, cancel = context.WithTimeout(parent, timeout)
	}

	copiedCtx := parent.ShallowCopy()
	copiedCtx.Context = ctx

	return copiedCtx, cancel
}
