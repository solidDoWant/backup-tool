package contexts

import "time"

type StopwatchContext struct {
	StartTime time.Time
}

func NewStopwatchContext() *StopwatchContext {
	return &StopwatchContext{
		StartTime: time.Now(),
	}
}

func (sc *StopwatchContext) Elapsed() time.Duration {
	return time.Since(sc.StartTime)
}

func (sc *StopwatchContext) Keyval() *DeferredKeyval {
	return NewDeferredKeyval("runtime", func() interface{} {
		return sc.Elapsed()
	})
}
