package contexts

import (
	"fmt"
	"io"
	"sync"

	"github.com/charmbracelet/log"
)

// This is a global logger that can be used when an initialized logger is
// needed, but no information about it has been specified. This is a good
// default value for *log.Logger fields.
var nullLogger = log.NewWithOptions(io.Discard, log.Options{
	Level: log.FatalLevel,
})

type DeferredKeyvalInterface interface {
	Keyval() []interface{}
}

type DeferredKeyval struct {
	Key   string
	Value func() interface{}
}

func NewDeferredKeyval(key string, value func() interface{}) *DeferredKeyval {
	return &DeferredKeyval{
		Key:   key,
		Value: value,
	}
}

func (dk *DeferredKeyval) Keyval() []interface{} {
	return []interface{}{dk.Key, dk.Value()}
}

type deferredErrorKeyvals struct {
	err *error
}

func (dek *deferredErrorKeyvals) Keyval() []interface{} {
	if dek.err == nil || *dek.err == nil {
		return nil
	}

	return []interface{}{"error", *dek.err}
}

// Creates a common keyval for a future error. If the error is nil, the keyval will be
// be omitted entirely.
func ErrorKeyvals(err *error) DeferredKeyvalInterface {
	return &deferredErrorKeyvals{err: err}
}

type LoggerContext struct {
	*log.Logger
	mu        *sync.RWMutex
	stepCount int
	// The underlying logger has support for prefixes, but it always places a
	// single ':' between the prefix and the message. This is a workaround for
	// that, allowing for prefixes to be added without a ':'.
	prefix string
}

func NewLoggerContext(logger *log.Logger) *LoggerContext {
	return &LoggerContext{
		Logger: logger,
		mu:     &sync.RWMutex{},
	}
}

// Create a new logger context that is a child of the current logger context.
func (lc *LoggerContext) child() *LoggerContext {
	newLogger := NewLoggerContext(lc.Logger.With())

	lc.mu.RLock()
	parentPrefix := lc.prefix
	parentStepCount := lc.stepCount
	lc.mu.RUnlock()

	newLogger.prefix = getStepPrefix(parentPrefix, parentStepCount) + ":::"
	return newLogger
}

func (lc *LoggerContext) SetPrefix(prefix string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.prefix = prefix
}

func (lc *LoggerContext) GetPrefix() string {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	return lc.prefix
}

// Simple wrapper for the underlying logger `With` that returns the logger context.
func (lc *LoggerContext) With(keyvals ...interface{}) *LoggerContext {
	lc.Logger = lc.Logger.With(keyvals...)
	return lc
}

// This takes prefix and step count as an input so that the caller can decide
// what locking is needed, if any.
func getStepPrefix(prefix string, stepCount int) string {
	if stepCount > 0 {
		return fmt.Sprintf("%s(%d)", prefix, stepCount)
	}
	return prefix
}

// Creates a new logger instance with the same logger as the current logger, but
// with a new prefix that includes the step count.
// Successive calls to Step will increment the step count.
func (lc *LoggerContext) Step() *LoggerContext {
	lc.mu.Lock()
	lc.stepCount++
	prefix := lc.prefix
	stepCount := lc.stepCount
	lc.mu.Unlock()

	newLogger := NewLoggerContext(lc.Logger.With())
	newLogger.prefix = getStepPrefix(prefix, stepCount)
	return newLogger
}

// Take a slice of keyvals and process any DeferredKeyvals, returning a new
// slice with the DeferredKeyvals expanded. Order is preserved.
func processDeferredKeyvals(keyvals []interface{}) []interface{} {
	processedKeyvals := make([]interface{}, 0, len(keyvals))
	for _, keyval := range keyvals {
		if dk, ok := keyval.(DeferredKeyvalInterface); ok {
			processedKeyvals = append(processedKeyvals, dk.Keyval()...)
		} else {
			processedKeyvals = append(processedKeyvals, keyval)
		}
	}
	return processedKeyvals
}

// Process a logf-based call, changing the parameters as needed to support the custom functionality
// of this logger. The input should be logf-style, with a format string and a slice of arguments.
// The outputs will be the processed version of these arguments.
func (lc *LoggerContext) processLogfCall(msg string, args []interface{}) (string, []interface{}) {
	args = processDeferredKeyvals(args)
	prefix := lc.GetPrefix()
	if prefix != "" {
		msg = fmt.Sprintf("%s %s", prefix, msg)
	}
	return msg, args
}

// Process a log-based call, changing the parameters as needed to support the custom functionality
// of this logger. The input should be log-style, with a message and a slice of keyvals. The outputs
// will be the processed version of these arguments.
func (lc *LoggerContext) processLogCall(msg interface{}, keyvals []interface{}) (interface{}, []interface{}) {
	formattedMessage := ""
	if msg != nil {
		formattedMessage = fmt.Sprint(msg)
	}

	return lc.processLogfCall(formattedMessage, keyvals)
}

// These are all basically the same function, but are needed to override the
// log.Logger methods. Unforunately, the logger library doesn't provide a "hook"
// for this, which would greatly reduce the boilerplate code here.

func (lc *LoggerContext) Log(level log.Level, msg interface{}, keyvals ...interface{}) {
	lc.Helper() // Needed so that the formatter uses the correct stack frame
	msg, keyvals = lc.processLogCall(msg, keyvals)
	lc.Logger.Log(level, msg, keyvals...)
}

func (lc *LoggerContext) Debug(msg interface{}, keyvals ...interface{}) {
	lc.Helper()
	msg, keyvals = lc.processLogCall(msg, keyvals)
	lc.Logger.Debug(msg, keyvals...)
}

func (lc *LoggerContext) Info(msg interface{}, keyvals ...interface{}) {
	lc.Helper()
	msg, keyvals = lc.processLogCall(msg, keyvals)
	lc.Logger.Info(msg, keyvals...)
}

func (lc *LoggerContext) Warn(msg interface{}, keyvals ...interface{}) {
	lc.Helper()
	msg, keyvals = lc.processLogCall(msg, keyvals)
	lc.Logger.Warn(msg, keyvals...)
}

func (lc *LoggerContext) Error(msg interface{}, keyvals ...interface{}) {
	lc.Helper()
	msg, keyvals = lc.processLogCall(msg, keyvals)
	lc.Logger.Error(msg, keyvals...)
}

func (lc *LoggerContext) Fatal(msg interface{}, keyvals ...interface{}) {
	lc.Helper()
	msg, keyvals = lc.processLogCall(msg, keyvals)
	lc.Logger.Fatal(msg, keyvals...)
}

func (lc *LoggerContext) Print(msg interface{}, keyvals ...interface{}) {
	lc.Helper()
	msg, keyvals = lc.processLogCall(msg, keyvals)
	lc.Logger.Print(msg, keyvals...)
}

func (lc *LoggerContext) Logf(level log.Level, format string, args ...interface{}) {
	lc.Helper()
	format, args = lc.processLogfCall(format, args)
	lc.Logger.Logf(level, format, args...)
}

func (lc *LoggerContext) Debugf(format string, args ...interface{}) {
	lc.Helper()
	format, args = lc.processLogfCall(format, args)
	lc.Logger.Debugf(format, args...)
}

func (lc *LoggerContext) Infof(format string, args ...interface{}) {
	lc.Helper()
	format, args = lc.processLogfCall(format, args)
	lc.Logger.Infof(format, args...)
}

func (lc *LoggerContext) Warnf(format string, args ...interface{}) {
	lc.Helper()
	format, args = lc.processLogfCall(format, args)
	lc.Logger.Warnf(format, args...)
}

func (lc *LoggerContext) Errorf(format string, args ...interface{}) {
	lc.Helper()
	format, args = lc.processLogfCall(format, args)
	lc.Logger.Errorf(format, args...)
}

func (lc *LoggerContext) Fatalf(format string, args ...interface{}) {
	lc.Helper()
	format, args = lc.processLogfCall(format, args)
	lc.Logger.Fatalf(format, args...)
}

func (lc *LoggerContext) Printf(format string, args ...interface{}) {
	lc.Helper()
	format, args = lc.processLogfCall(format, args)
	lc.Logger.Printf(format, args...)
}
