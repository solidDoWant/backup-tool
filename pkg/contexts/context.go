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
	Log       *LoggerContext
	Stopwatch *StopwatchContext
	parentCtx *Context // The parent context, if there is one.
}

func NewContext(ctx context.Context) *Context {
	newCtx := &Context{
		Context:   ctx,
		Stopwatch: NewStopwatchContext(),
		Log:       NewLoggerContext(nullLogger),
	}

	if parentCtx, ok := ctx.(*Context); ok {
		newCtx.parentCtx = parentCtx
	}

	return newCtx
}

func (c *Context) WithLogger(logger *LoggerContext) *Context {
	c.Log = logger
	return c
}

func (c *Context) Child() *Context {
	childCtx := *c
	childCtx.Stopwatch = NewStopwatchContext()
	childCtx.Log = c.Log.child()
	childCtx.parentCtx = c

	return &childCtx
}

// Returns a new context with the given timeout. If the timeout is 0, the new
// context will be cancellable, but will not have a timeout.
func (c *Context) WithTimeout(timeout time.Duration) (*Context, context.CancelFunc) {
	var ctx context.Context
	var cancel context.CancelFunc
	if timeout == 0 {
		ctx, cancel = context.WithCancel(c.Context)
	} else {
		ctx, cancel = context.WithTimeout(c.Context, timeout)
	}

	c.Context = ctx

	return c, cancel
}

func (c *Context) IsChildOf(maybeParentCtx *Context) bool {
	if c.parentCtx == nil {
		return false
	}

	if c.parentCtx == maybeParentCtx {
		return true
	}

	return c.parentCtx.IsChildOf(maybeParentCtx)
}

// These are a workaround to provide the serve context to handler functions. Wrap the handler context
// with the real context, and return the real context as the stdlib context type.
func WrapHandlerContext(handlerCtx context.Context, realCtx *Context) context.Context {
	realCtx.Context = handlerCtx
	return realCtx
}

// Unwrap the real context from the stdlib context type. If the context was not properly attached, return a new context
// with a message about the issue.
func UnwrapHandlerContext(ctx context.Context) *Context {
	if casted, ok := ctx.(*Context); ok {
		return casted
	}

	// If this is hit, it means the context was not properly attached (which is a bug).
	// Usually this isn't a critical issue, so return a new context with a message about the issue.
	bugCtx := NewContext(ctx)
	bugCtx.Log.With("bug", "context was not properly attached").Error("context was not properly attached")
	return bugCtx
}
