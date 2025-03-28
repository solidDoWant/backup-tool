// Code generated by mockery v2.51.0. DO NOT EDIT.

package clonepvc

import (
	contexts "github.com/solidDoWant/backup-tool/pkg/contexts"
	mock "github.com/stretchr/testify/mock"

	v1 "k8s.io/api/core/v1"
)

// MockProviderInterface is an autogenerated mock type for the ProviderInterface type
type MockProviderInterface struct {
	mock.Mock
}

type MockProviderInterface_Expecter struct {
	mock *mock.Mock
}

func (_m *MockProviderInterface) EXPECT() *MockProviderInterface_Expecter {
	return &MockProviderInterface_Expecter{mock: &_m.Mock}
}

// ClonePVC provides a mock function with given fields: ctx, namespace, pvcName, opts
func (_m *MockProviderInterface) ClonePVC(ctx *contexts.Context, namespace string, pvcName string, opts ClonePVCOptions) (*v1.PersistentVolumeClaim, error) {
	ret := _m.Called(ctx, namespace, pvcName, opts)

	if len(ret) == 0 {
		panic("no return value specified for ClonePVC")
	}

	var r0 *v1.PersistentVolumeClaim
	var r1 error
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, ClonePVCOptions) (*v1.PersistentVolumeClaim, error)); ok {
		return rf(ctx, namespace, pvcName, opts)
	}
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, ClonePVCOptions) *v1.PersistentVolumeClaim); ok {
		r0 = rf(ctx, namespace, pvcName, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.PersistentVolumeClaim)
		}
	}

	if rf, ok := ret.Get(1).(func(*contexts.Context, string, string, ClonePVCOptions) error); ok {
		r1 = rf(ctx, namespace, pvcName, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockProviderInterface_ClonePVC_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ClonePVC'
type MockProviderInterface_ClonePVC_Call struct {
	*mock.Call
}

// ClonePVC is a helper method to define mock.On call
//   - ctx *contexts.Context
//   - namespace string
//   - pvcName string
//   - opts ClonePVCOptions
func (_e *MockProviderInterface_Expecter) ClonePVC(ctx interface{}, namespace interface{}, pvcName interface{}, opts interface{}) *MockProviderInterface_ClonePVC_Call {
	return &MockProviderInterface_ClonePVC_Call{Call: _e.mock.On("ClonePVC", ctx, namespace, pvcName, opts)}
}

func (_c *MockProviderInterface_ClonePVC_Call) Run(run func(ctx *contexts.Context, namespace string, pvcName string, opts ClonePVCOptions)) *MockProviderInterface_ClonePVC_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*contexts.Context), args[1].(string), args[2].(string), args[3].(ClonePVCOptions))
	})
	return _c
}

func (_c *MockProviderInterface_ClonePVC_Call) Return(clonedPvc *v1.PersistentVolumeClaim, err error) *MockProviderInterface_ClonePVC_Call {
	_c.Call.Return(clonedPvc, err)
	return _c
}

func (_c *MockProviderInterface_ClonePVC_Call) RunAndReturn(run func(*contexts.Context, string, string, ClonePVCOptions) (*v1.PersistentVolumeClaim, error)) *MockProviderInterface_ClonePVC_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockProviderInterface creates a new instance of MockProviderInterface. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockProviderInterface(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockProviderInterface {
	mock := &MockProviderInterface{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
