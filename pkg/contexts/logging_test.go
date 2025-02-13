package contexts

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeferredKeyval(t *testing.T) {
	assert.Implements(t, (*DeferredKeyvalInterface)(nil), &DeferredKeyval{})
}

func TestDeferredKeyvalKeyval(t *testing.T) {
	dk := NewDeferredKeyval("key", func() interface{} { return "value" })
	assert.Equal(t, []interface{}{"key", "value"}, dk.Keyval())
}

func TestDeferredErrorKeyvals(t *testing.T) {
	assert.Implements(t, (*DeferredKeyvalInterface)(nil), &deferredErrorKeyvals{})
}

func TestDeferredErrorKeyvalsKeyval(t *testing.T) {
	err := assert.AnError
	deferredKeyvals := &deferredErrorKeyvals{err: &err}
	assert.Equal(t, []interface{}{"error", err}, deferredKeyvals.Keyval())

	err = nil
	deferredKeyvals = &deferredErrorKeyvals{err: &err}
	assert.Nil(t, deferredKeyvals.Keyval())

	deferredKeyvals = &deferredErrorKeyvals{}
	assert.Nil(t, deferredKeyvals.Keyval())
}

func TestErrorKeyvals(t *testing.T) {
	err := assert.AnError
	deferredKeyvals := ErrorKeyvals(&err)
	assert.NotNil(t, deferredKeyvals)

	err = nil
	deferredKeyvals = ErrorKeyvals(&err)
	assert.NotNil(t, deferredKeyvals)

	deferredKeyvals = ErrorKeyvals(nil)
	assert.NotNil(t, deferredKeyvals)
}

func TestNewLoggerContext(t *testing.T) {
	underlyingLogger := log.Default()
	logger := NewLoggerContext(underlyingLogger)
	require.NotNil(t, logger)
	assert.Equal(t, underlyingLogger, logger.Logger)
	assert.NotNil(t, logger.mu)
	assert.Zero(t, logger.stepCount)
	assert.Empty(t, logger.prefix)
}

func TestLoggerContextChild(t *testing.T) {
	var underlyingWriter bytes.Buffer
	underlyingLogger := log.New(&underlyingWriter).With("key", "value")
	logger := NewLoggerContext(underlyingLogger)

	child := logger.child()
	require.NotNil(t, child)
	assert.NotNil(t, child.mu)
	assert.NotSame(t, logger.mu, child.mu)
	assert.Zero(t, child.stepCount)
	assert.True(t, strings.HasSuffix(child.prefix, ":::"))

	// Use this as a check to verify that the underlying loggers are equal,
	// though they still need separate mutexes
	child.Info("test")
	childLoggedField := underlyingWriter.String()
	assert.Equal(t, "INFO ::: test key=value\n", childLoggedField)

	underlyingWriter.Reset()
	child2 := child.child()
	child2.Warn("test2")
	child2LoggedField := underlyingWriter.String()
	assert.Equal(t, "WARN :::::: test2 key=value\n", child2LoggedField)
}

func TestLoggerContextSetPrefix(t *testing.T) {
	logger := NewLoggerContext(log.Default())

	// Test setting prefix
	logger.SetPrefix("test-prefix")
	assert.Equal(t, "test-prefix", logger.GetPrefix())

	// Test overwriting existing prefix
	logger.SetPrefix("new-prefix")
	assert.Equal(t, "new-prefix", logger.GetPrefix())

	// Test setting empty prefix
	logger.SetPrefix("")
	assert.Empty(t, logger.GetPrefix())
}

func TestLoggerContextGetPrefix(t *testing.T) {
	logger := NewLoggerContext(log.Default())

	// Test getting empty prefix
	assert.Empty(t, logger.GetPrefix())

	// Test getting set prefix
	logger.SetPrefix("test-prefix")
	assert.Equal(t, "test-prefix", logger.GetPrefix())
}

func TestLoggerContextWith(t *testing.T) {
	var underlyingWriter bytes.Buffer
	underlyingLogger := log.New(&underlyingWriter)
	logger := NewLoggerContext(underlyingLogger)

	// Test adding keyvals
	loggerWithKeyvals := logger.With("key", "value", "key2", "value2")
	require.NotNil(t, loggerWithKeyvals)
	assert.Same(t, logger, loggerWithKeyvals) // Should return same instance

	// Verify keyvals were added by logging a message
	loggerWithKeyvals.Info("test")
	loggedOutput := underlyingWriter.String()
	assert.Equal(t, "INFO test key=value key2=value2\n", loggedOutput)

	// Test chaining With calls
	underlyingWriter.Reset()
	loggerWithMoreKeyvals := loggerWithKeyvals.With("key3", "value3")
	loggerWithMoreKeyvals.Info("test2")
	loggedOutput = underlyingWriter.String()
	assert.Equal(t, "INFO test2 key=value key2=value2 key3=value3\n", loggedOutput)
}

func TestGetStepPrefix(t *testing.T) {
	tests := []struct {
		desc      string
		prefix    string
		stepCount int
		want      string
	}{
		{
			desc: "empty prefix with zero step count",
		},
		{
			desc:   "zero step count returns original prefix",
			prefix: "test",
			want:   "test",
		},
		{
			desc:      "positive step count appends number",
			prefix:    "test",
			stepCount: 1,
			want:      "test(1)",
		},
		{
			desc:      "empty prefix with step count",
			stepCount: 2,
			want:      "(2)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := getStepPrefix(tt.prefix, tt.stepCount)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLoggerContextStep(t *testing.T) {
	var underlyingWriter bytes.Buffer
	underlyingLogger := log.New(&underlyingWriter)
	logger := NewLoggerContext(underlyingLogger)

	// First step should create new logger with step count 1
	step1 := logger.Step()
	require.NotNil(t, step1)
	assert.NotSame(t, logger, step1)
	step1.Info("test1")
	assert.Equal(t, "INFO (1) test1\n", underlyingWriter.String())

	// Multiple steps should increment counter
	underlyingWriter.Reset()
	step2 := logger.Step()
	step2.Info("test2")
	assert.Equal(t, "INFO (2) test2\n", underlyingWriter.String())

	// Steps with prefix
	underlyingWriter.Reset()
	logger.SetPrefix("prefix")
	step3 := logger.Step()
	step3.Info("test3")
	assert.Equal(t, "INFO prefix(3) test3\n", underlyingWriter.String())

	// Stepping from stepped logger starts new count
	underlyingWriter.Reset()
	step3Child := step3.Step()
	step3Child.Info("test4")
	assert.Equal(t, "INFO prefix(3)(1) test4\n", underlyingWriter.String())
}

func TestProcessDeferredKeyvals(t *testing.T) {
	tests := []struct {
		desc    string
		keyvals []interface{}
		want    []interface{}
	}{
		{
			desc:    "empty input",
			keyvals: []interface{}{},
			want:    []interface{}{},
		},
		{
			desc:    "non-deferred keyvals",
			keyvals: []interface{}{"key", "value", "key2", 123},
			want:    []interface{}{"key", "value", "key2", 123},
		},
		{
			desc: "deferred keyval",
			keyvals: []interface{}{
				NewDeferredKeyval("key", func() interface{} { return "value" }),
			},
			want: []interface{}{"key", "value"},
		},
		{
			desc: "mixed keyvals",
			keyvals: []interface{}{
				"regular", "value",
				NewDeferredKeyval("deferred", func() interface{} { return 123 }),
			},
			want: []interface{}{"regular", "value", "deferred", 123},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := processDeferredKeyvals(tt.keyvals)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLoggerContextProcessLogfCall(t *testing.T) {
	tests := []struct {
		desc     string
		prefix   string
		msg      string
		args     []interface{}
		wantMsg  string
		wantArgs []interface{}
	}{
		{
			desc:     "empty prefix and args",
			msg:      "test message",
			wantMsg:  "test message",
			wantArgs: []interface{}{},
		},
		{
			desc:     "with prefix",
			prefix:   "PREFIX",
			msg:      "test message",
			wantMsg:  "PREFIX test message",
			wantArgs: []interface{}{},
		},
		{
			desc:     "with regular args",
			msg:      "test message",
			args:     []interface{}{"key", "value"},
			wantMsg:  "test message",
			wantArgs: []interface{}{"key", "value"},
		},
		{
			desc: "with deferred args",
			msg:  "test message",
			args: []interface{}{
				NewDeferredKeyval("key", func() interface{} { return "value" }),
			},
			wantMsg:  "test message",
			wantArgs: []interface{}{"key", "value"},
		},
		{
			desc:     "with prefix and args",
			prefix:   "PREFIX",
			msg:      "test message",
			args:     []interface{}{"key", "value"},
			wantMsg:  "PREFIX test message",
			wantArgs: []interface{}{"key", "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			logger := NewLoggerContext(log.Default())
			logger.SetPrefix(tt.prefix)

			gotMsg, gotArgs := logger.processLogfCall(tt.msg, tt.args)

			assert.Equal(t, tt.wantMsg, gotMsg)
			assert.Equal(t, tt.wantArgs, gotArgs)
		})
	}
}

func TestLoggerContextProcessLogCall(t *testing.T) {
	tests := []struct {
		desc     string
		prefix   string
		msg      interface{}
		keyvals  []interface{}
		wantMsg  interface{}
		wantKeys []interface{}
	}{
		{
			desc:     "nil message",
			wantKeys: []interface{}{},
			wantMsg:  "",
		},
		{
			desc:     "string message",
			msg:      "test message",
			wantMsg:  "test message",
			wantKeys: []interface{}{},
		},
		{
			desc:     "with prefix",
			prefix:   "PREFIX",
			msg:      "test message",
			wantMsg:  "PREFIX test message",
			wantKeys: []interface{}{},
		},
		{
			desc:     "with keyvals",
			msg:      "test message",
			keyvals:  []interface{}{"key", "value"},
			wantMsg:  "test message",
			wantKeys: []interface{}{"key", "value"},
		},
		{
			desc: "with deferred keyvals",
			msg:  "test message",
			keyvals: []interface{}{
				NewDeferredKeyval("key", func() interface{} { return "value" }),
			},
			wantMsg:  "test message",
			wantKeys: []interface{}{"key", "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			logger := NewLoggerContext(log.Default())
			logger.SetPrefix(tt.prefix)

			gotMsg, gotKeys := logger.processLogCall(tt.msg, tt.keyvals)

			assert.Equal(t, tt.wantMsg, gotMsg)
			assert.Equal(t, tt.wantKeys, gotKeys)
		})
	}
}

func TestLoggerContextLogFuncs(t *testing.T) {
	var underlyingWriter bytes.Buffer
	underlyingLogger := log.New(&underlyingWriter)
	underlyingLogger.SetLevel(log.DebugLevel)
	logger := NewLoggerContext(underlyingLogger)

	// Test log
	logger.Log(log.InfoLevel, "test message")
	assert.Equal(t, "INFO test message\n", underlyingWriter.String())

	// Test Debug
	underlyingWriter.Reset()
	logger.Debug("test message")
	assert.Equal(t, "DEBU test message\n", underlyingWriter.String())

	// Test Info
	underlyingWriter.Reset()
	logger.Info("test message")
	assert.Equal(t, "INFO test message\n", underlyingWriter.String())

	// Test Warn
	underlyingWriter.Reset()
	logger.Warn("test message")
	assert.Equal(t, "WARN test message\n", underlyingWriter.String())

	// Test Error
	underlyingWriter.Reset()
	logger.Error("test message")
	assert.Equal(t, "ERRO test message\n", underlyingWriter.String())

	// Test Print
	underlyingWriter.Reset()
	logger.Print("test message")
	assert.Equal(t, "test message\n", underlyingWriter.String())
}

func TestLoggerContextLogfFuncs(t *testing.T) {
	var underlyingWriter bytes.Buffer
	underlyingLogger := log.New(&underlyingWriter)
	underlyingLogger.SetLevel(log.DebugLevel)
	logger := NewLoggerContext(underlyingLogger)

	// Test Logf
	logger.Logf(log.InfoLevel, "test %s", "message")
	assert.Equal(t, "INFO test message\n", underlyingWriter.String())

	// Test Debugf
	underlyingWriter.Reset()
	logger.Debugf("test %s", "message")
	assert.Equal(t, "DEBU test message\n", underlyingWriter.String())

	// Test Infof
	underlyingWriter.Reset()
	logger.Infof("test %s", "message")
	assert.Equal(t, "INFO test message\n", underlyingWriter.String())

	// Test Warnf
	underlyingWriter.Reset()
	logger.Warnf("test %s", "message")
	assert.Equal(t, "WARN test message\n", underlyingWriter.String())

	// Test Errorf
	underlyingWriter.Reset()
	logger.Errorf("test %s", "message")
	assert.Equal(t, "ERRO test message\n", underlyingWriter.String())

	// Test Printf
	underlyingWriter.Reset()
	logger.Printf("test %s", "message")
	assert.Equal(t, "test message\n", underlyingWriter.String())
}
