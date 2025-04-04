// Code generated by mockery v2.51.0. DO NOT EDIT.

package features

import (
	context "context"

	cobra "github.com/spf13/cobra"

	contexts "github.com/solidDoWant/backup-tool/pkg/contexts"

	mock "github.com/stretchr/testify/mock"
)

// MockContextCommandInterface is an autogenerated mock type for the ContextCommandInterface type
type MockContextCommandInterface struct {
	mock.Mock
}

type MockContextCommandInterface_Expecter struct {
	mock *mock.Mock
}

func (_m *MockContextCommandInterface) EXPECT() *MockContextCommandInterface_Expecter {
	return &MockContextCommandInterface_Expecter{mock: &_m.Mock}
}

// ConfigureFlags provides a mock function with given fields: cmd
func (_m *MockContextCommandInterface) ConfigureFlags(cmd *cobra.Command) {
	_m.Called(cmd)
}

// MockContextCommandInterface_ConfigureFlags_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ConfigureFlags'
type MockContextCommandInterface_ConfigureFlags_Call struct {
	*mock.Call
}

// ConfigureFlags is a helper method to define mock.On call
//   - cmd *cobra.Command
func (_e *MockContextCommandInterface_Expecter) ConfigureFlags(cmd interface{}) *MockContextCommandInterface_ConfigureFlags_Call {
	return &MockContextCommandInterface_ConfigureFlags_Call{Call: _e.mock.On("ConfigureFlags", cmd)}
}

func (_c *MockContextCommandInterface_ConfigureFlags_Call) Run(run func(cmd *cobra.Command)) *MockContextCommandInterface_ConfigureFlags_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*cobra.Command))
	})
	return _c
}

func (_c *MockContextCommandInterface_ConfigureFlags_Call) Return() *MockContextCommandInterface_ConfigureFlags_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockContextCommandInterface_ConfigureFlags_Call) RunAndReturn(run func(*cobra.Command)) *MockContextCommandInterface_ConfigureFlags_Call {
	_c.Run(run)
	return _c
}

// GetCommandContext provides a mock function with no fields
func (_m *MockContextCommandInterface) GetCommandContext() (*contexts.Context, context.CancelFunc) {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetCommandContext")
	}

	var r0 *contexts.Context
	var r1 context.CancelFunc
	if rf, ok := ret.Get(0).(func() (*contexts.Context, context.CancelFunc)); ok {
		return rf()
	}
	if rf, ok := ret.Get(0).(func() *contexts.Context); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*contexts.Context)
		}
	}

	if rf, ok := ret.Get(1).(func() context.CancelFunc); ok {
		r1 = rf()
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(context.CancelFunc)
		}
	}

	return r0, r1
}

// MockContextCommandInterface_GetCommandContext_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetCommandContext'
type MockContextCommandInterface_GetCommandContext_Call struct {
	*mock.Call
}

// GetCommandContext is a helper method to define mock.On call
func (_e *MockContextCommandInterface_Expecter) GetCommandContext() *MockContextCommandInterface_GetCommandContext_Call {
	return &MockContextCommandInterface_GetCommandContext_Call{Call: _e.mock.On("GetCommandContext")}
}

func (_c *MockContextCommandInterface_GetCommandContext_Call) Run(run func()) *MockContextCommandInterface_GetCommandContext_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockContextCommandInterface_GetCommandContext_Call) Return(_a0 *contexts.Context, _a1 context.CancelFunc) *MockContextCommandInterface_GetCommandContext_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockContextCommandInterface_GetCommandContext_Call) RunAndReturn(run func() (*contexts.Context, context.CancelFunc)) *MockContextCommandInterface_GetCommandContext_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockContextCommandInterface creates a new instance of MockContextCommandInterface. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockContextCommandInterface(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockContextCommandInterface {
	mock := &MockContextCommandInterface{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
