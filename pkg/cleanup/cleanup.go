package cleanup

import (
	"context"
	"time"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
)

var DefaultCleanupTimeout = 10 * time.Minute

// Cleanup is a type that can be used to run deferred cleanup logic with error handling.
// If the timeout is not set, it will default to 10 minutes.
// Cleanup logic uses the background context to ensure it runs even if the original context was canceled.
// Usage: defer cleanup.To(func() error { return nil }).WithErrMessage("some message").WithOriginalErr(&err).Done()
type Cleanup struct {
	cleanupLogic func(*contexts.Context) error
	onErrArgs    []interface{}
	originalErr  *error
	parentCtx    *contexts.Context
	timeout      time.Duration
}

func To(cleanupLogic func(*contexts.Context) error) *Cleanup {
	return &Cleanup{
		cleanupLogic: cleanupLogic,
	}
}

func (c *Cleanup) WithErrMessage(errMessage string, args ...interface{}) *Cleanup {
	c.onErrArgs = append([]interface{}{errMessage}, args...)
	return c
}

func (c *Cleanup) WithOriginalErr(err *error) *Cleanup {
	c.originalErr = err
	return c
}

func (c *Cleanup) WithParentCtx(ctx *contexts.Context) *Cleanup {
	c.parentCtx = ctx
	return c
}

func (c *Cleanup) WithTimeout(timeout time.Duration) *Cleanup {
	c.timeout = timeout
	return c
}

func (c *Cleanup) buildContext() (*contexts.Context, context.CancelFunc) {
	// This must use a new context to ensure that the cleanup logic runs even if the original context is canceled.
	// Some properties of the parent context (such as logger) should be reused.
	newCtx := contexts.NewContext(context.Background())
	if c.parentCtx != nil {
		newCtx.Log = contexts.NewLoggerContext(c.parentCtx.Log.Logger.With())
		newCtx.Log.SetPrefix(c.parentCtx.Log.GetPrefix())
	}

	timeout := c.timeout
	if timeout == 0 {
		timeout = DefaultCleanupTimeout
	}
	_, cancel := newCtx.WithTimeout(timeout)

	newCtx.Log.SetPrefix(newCtx.Log.GetPrefix() + "***")

	return newCtx, cancel
}

func (c *Cleanup) Run() error {
	if c.cleanupLogic == nil {
		return nil
	}

	cleanupCtx, cancel := c.buildContext()
	defer cancel()

	cleanupErr := c.cleanupLogic(cleanupCtx)
	cleanupErr = trace.Wrap(cleanupErr, c.onErrArgs...)
	aggregateErr := cleanupErr
	if c.originalErr != nil {
		aggregateErr = trace.NewAggregate(*c.originalErr, cleanupErr)
		*c.originalErr = aggregateErr
	}

	return aggregateErr
}
