// Code generated by mockery v2.51.0. DO NOT EDIT.

package helpers

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
	runtime "k8s.io/apimachinery/pkg/runtime"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	watch "k8s.io/apimachinery/pkg/watch"
)

// MockListerWatcher is an autogenerated mock type for the ListerWatcher type
type MockListerWatcher[TList runtime.Object] struct {
	mock.Mock
}

type MockListerWatcher_Expecter[TList runtime.Object] struct {
	mock *mock.Mock
}

func (_m *MockListerWatcher[TList]) EXPECT() *MockListerWatcher_Expecter[TList] {
	return &MockListerWatcher_Expecter[TList]{mock: &_m.Mock}
}

// List provides a mock function with given fields: ctx, opts
func (_m *MockListerWatcher[TList]) List(ctx context.Context, opts v1.ListOptions) (TList, error) {
	ret := _m.Called(ctx, opts)

	if len(ret) == 0 {
		panic("no return value specified for List")
	}

	var r0 TList
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, v1.ListOptions) (TList, error)); ok {
		return rf(ctx, opts)
	}
	if rf, ok := ret.Get(0).(func(context.Context, v1.ListOptions) TList); ok {
		r0 = rf(ctx, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(TList)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, v1.ListOptions) error); ok {
		r1 = rf(ctx, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockListerWatcher_List_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'List'
type MockListerWatcher_List_Call[TList runtime.Object] struct {
	*mock.Call
}

// List is a helper method to define mock.On call
//   - ctx context.Context
//   - opts v1.ListOptions
func (_e *MockListerWatcher_Expecter[TList]) List(ctx interface{}, opts interface{}) *MockListerWatcher_List_Call[TList] {
	return &MockListerWatcher_List_Call[TList]{Call: _e.mock.On("List", ctx, opts)}
}

func (_c *MockListerWatcher_List_Call[TList]) Run(run func(ctx context.Context, opts v1.ListOptions)) *MockListerWatcher_List_Call[TList] {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(v1.ListOptions))
	})
	return _c
}

func (_c *MockListerWatcher_List_Call[TList]) Return(_a0 TList, _a1 error) *MockListerWatcher_List_Call[TList] {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockListerWatcher_List_Call[TList]) RunAndReturn(run func(context.Context, v1.ListOptions) (TList, error)) *MockListerWatcher_List_Call[TList] {
	_c.Call.Return(run)
	return _c
}

// Watch provides a mock function with given fields: ctx, opts
func (_m *MockListerWatcher[TList]) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	ret := _m.Called(ctx, opts)

	if len(ret) == 0 {
		panic("no return value specified for Watch")
	}

	var r0 watch.Interface
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, v1.ListOptions) (watch.Interface, error)); ok {
		return rf(ctx, opts)
	}
	if rf, ok := ret.Get(0).(func(context.Context, v1.ListOptions) watch.Interface); ok {
		r0 = rf(ctx, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(watch.Interface)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, v1.ListOptions) error); ok {
		r1 = rf(ctx, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockListerWatcher_Watch_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Watch'
type MockListerWatcher_Watch_Call[TList runtime.Object] struct {
	*mock.Call
}

// Watch is a helper method to define mock.On call
//   - ctx context.Context
//   - opts v1.ListOptions
func (_e *MockListerWatcher_Expecter[TList]) Watch(ctx interface{}, opts interface{}) *MockListerWatcher_Watch_Call[TList] {
	return &MockListerWatcher_Watch_Call[TList]{Call: _e.mock.On("Watch", ctx, opts)}
}

func (_c *MockListerWatcher_Watch_Call[TList]) Run(run func(ctx context.Context, opts v1.ListOptions)) *MockListerWatcher_Watch_Call[TList] {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(v1.ListOptions))
	})
	return _c
}

func (_c *MockListerWatcher_Watch_Call[TList]) Return(_a0 watch.Interface, _a1 error) *MockListerWatcher_Watch_Call[TList] {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockListerWatcher_Watch_Call[TList]) RunAndReturn(run func(context.Context, v1.ListOptions) (watch.Interface, error)) *MockListerWatcher_Watch_Call[TList] {
	_c.Call.Return(run)
	return _c
}

// NewMockListerWatcher creates a new instance of MockListerWatcher. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockListerWatcher[TList runtime.Object](t interface {
	mock.TestingT
	Cleanup(func())
}) *MockListerWatcher[TList] {
	mock := &MockListerWatcher[TList]{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
