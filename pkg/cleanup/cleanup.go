package cleanup

import (
	"context"
	"time"

	"github.com/gravitational/trace"
)

var DefaultCleanupTimeout = 10 * time.Minute

// Cleanup is a type that can be used to run deferred cleanup logic with error handling.
// If the timeout is not set, it will default to 10 minutes.
// Cleanup logic uses the background context to ensure it runs even if the original context was canceled.
// Usage: defer cleanup.To(func() error { return nil }).WithErrMessage("some message").WithOriginalErr(&err).Done()
type Cleanup struct {
	cleanupLogic func() error
	onErrArgs    []interface{}
	originalErr  *error
}

func To(cleanupLogic func() error) *Cleanup {
	return &Cleanup{
		cleanupLogic: cleanupLogic,
	}
}

func WithTimeoutTo(timeout time.Duration, cleanupLogic func(context.Context) error) *Cleanup {
	return &Cleanup{
		cleanupLogic: func() error {
			if timeout == 0 {
				timeout = DefaultCleanupTimeout
			}

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			return cleanupLogic(ctx)
		},
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

func (c *Cleanup) Run() error {
	if c.cleanupLogic == nil {
		return nil
	}

	cleanupErr := c.cleanupLogic()
	cleanupErr = trace.Wrap(cleanupErr, c.onErrArgs...)
	aggregateErr := cleanupErr
	if c.originalErr != nil {
		aggregateErr = trace.NewAggregate(*c.originalErr, cleanupErr)
		*c.originalErr = aggregateErr
	}

	return aggregateErr
}
