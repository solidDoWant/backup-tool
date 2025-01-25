// Code generated by mockery v2.51.0. DO NOT EDIT.

package files

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
)

// MockRuntime is an autogenerated mock type for the Runtime type
type MockRuntime struct {
	mock.Mock
}

type MockRuntime_Expecter struct {
	mock *mock.Mock
}

func (_m *MockRuntime) EXPECT() *MockRuntime_Expecter {
	return &MockRuntime_Expecter{mock: &_m.Mock}
}

// CopyFiles provides a mock function with given fields: ctx, src, dest
func (_m *MockRuntime) CopyFiles(ctx context.Context, src string, dest string) error {
	ret := _m.Called(ctx, src, dest)

	if len(ret) == 0 {
		panic("no return value specified for CopyFiles")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string) error); ok {
		r0 = rf(ctx, src, dest)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockRuntime_CopyFiles_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'CopyFiles'
type MockRuntime_CopyFiles_Call struct {
	*mock.Call
}

// CopyFiles is a helper method to define mock.On call
//   - ctx context.Context
//   - src string
//   - dest string
func (_e *MockRuntime_Expecter) CopyFiles(ctx interface{}, src interface{}, dest interface{}) *MockRuntime_CopyFiles_Call {
	return &MockRuntime_CopyFiles_Call{Call: _e.mock.On("CopyFiles", ctx, src, dest)}
}

func (_c *MockRuntime_CopyFiles_Call) Run(run func(ctx context.Context, src string, dest string)) *MockRuntime_CopyFiles_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string), args[2].(string))
	})
	return _c
}

func (_c *MockRuntime_CopyFiles_Call) Return(_a0 error) *MockRuntime_CopyFiles_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockRuntime_CopyFiles_Call) RunAndReturn(run func(context.Context, string, string) error) *MockRuntime_CopyFiles_Call {
	_c.Call.Return(run)
	return _c
}

// SyncFiles provides a mock function with given fields: ctx, src, dest
func (_m *MockRuntime) SyncFiles(ctx context.Context, src string, dest string) error {
	ret := _m.Called(ctx, src, dest)

	if len(ret) == 0 {
		panic("no return value specified for SyncFiles")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string) error); ok {
		r0 = rf(ctx, src, dest)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockRuntime_SyncFiles_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'SyncFiles'
type MockRuntime_SyncFiles_Call struct {
	*mock.Call
}

// SyncFiles is a helper method to define mock.On call
//   - ctx context.Context
//   - src string
//   - dest string
func (_e *MockRuntime_Expecter) SyncFiles(ctx interface{}, src interface{}, dest interface{}) *MockRuntime_SyncFiles_Call {
	return &MockRuntime_SyncFiles_Call{Call: _e.mock.On("SyncFiles", ctx, src, dest)}
}

func (_c *MockRuntime_SyncFiles_Call) Run(run func(ctx context.Context, src string, dest string)) *MockRuntime_SyncFiles_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string), args[2].(string))
	})
	return _c
}

func (_c *MockRuntime_SyncFiles_Call) Return(_a0 error) *MockRuntime_SyncFiles_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockRuntime_SyncFiles_Call) RunAndReturn(run func(context.Context, string, string) error) *MockRuntime_SyncFiles_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockRuntime creates a new instance of MockRuntime. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockRuntime(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockRuntime {
	mock := &MockRuntime{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
