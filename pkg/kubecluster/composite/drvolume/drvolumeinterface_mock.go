// Code generated by mockery v2.51.0. DO NOT EDIT.

package drvolume

import (
	contexts "github.com/solidDoWant/backup-tool/pkg/contexts"
	mock "github.com/stretchr/testify/mock"

	v1 "k8s.io/api/core/v1"
)

// MockDRVolumeInterface is an autogenerated mock type for the DRVolumeInterface type
type MockDRVolumeInterface struct {
	mock.Mock
}

type MockDRVolumeInterface_Expecter struct {
	mock *mock.Mock
}

func (_m *MockDRVolumeInterface) EXPECT() *MockDRVolumeInterface_Expecter {
	return &MockDRVolumeInterface_Expecter{mock: &_m.Mock}
}

// SnapshotAndWaitReady provides a mock function with given fields: ctx, snapshotName, opts
func (_m *MockDRVolumeInterface) SnapshotAndWaitReady(ctx *contexts.Context, snapshotName string, opts DRVolumeSnapshotAndWaitOptions) error {
	ret := _m.Called(ctx, snapshotName, opts)

	if len(ret) == 0 {
		panic("no return value specified for SnapshotAndWaitReady")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, DRVolumeSnapshotAndWaitOptions) error); ok {
		r0 = rf(ctx, snapshotName, opts)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockDRVolumeInterface_SnapshotAndWaitReady_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'SnapshotAndWaitReady'
type MockDRVolumeInterface_SnapshotAndWaitReady_Call struct {
	*mock.Call
}

// SnapshotAndWaitReady is a helper method to define mock.On call
//   - ctx *contexts.Context
//   - snapshotName string
//   - opts DRVolumeSnapshotAndWaitOptions
func (_e *MockDRVolumeInterface_Expecter) SnapshotAndWaitReady(ctx interface{}, snapshotName interface{}, opts interface{}) *MockDRVolumeInterface_SnapshotAndWaitReady_Call {
	return &MockDRVolumeInterface_SnapshotAndWaitReady_Call{Call: _e.mock.On("SnapshotAndWaitReady", ctx, snapshotName, opts)}
}

func (_c *MockDRVolumeInterface_SnapshotAndWaitReady_Call) Run(run func(ctx *contexts.Context, snapshotName string, opts DRVolumeSnapshotAndWaitOptions)) *MockDRVolumeInterface_SnapshotAndWaitReady_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*contexts.Context), args[1].(string), args[2].(DRVolumeSnapshotAndWaitOptions))
	})
	return _c
}

func (_c *MockDRVolumeInterface_SnapshotAndWaitReady_Call) Return(_a0 error) *MockDRVolumeInterface_SnapshotAndWaitReady_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockDRVolumeInterface_SnapshotAndWaitReady_Call) RunAndReturn(run func(*contexts.Context, string, DRVolumeSnapshotAndWaitOptions) error) *MockDRVolumeInterface_SnapshotAndWaitReady_Call {
	_c.Call.Return(run)
	return _c
}

// setPVC provides a mock function with given fields: pvc
func (_m *MockDRVolumeInterface) setPVC(pvc *v1.PersistentVolumeClaim) {
	_m.Called(pvc)
}

// MockDRVolumeInterface_setPVC_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'setPVC'
type MockDRVolumeInterface_setPVC_Call struct {
	*mock.Call
}

// setPVC is a helper method to define mock.On call
//   - pvc *v1.PersistentVolumeClaim
func (_e *MockDRVolumeInterface_Expecter) setPVC(pvc interface{}) *MockDRVolumeInterface_setPVC_Call {
	return &MockDRVolumeInterface_setPVC_Call{Call: _e.mock.On("setPVC", pvc)}
}

func (_c *MockDRVolumeInterface_setPVC_Call) Run(run func(pvc *v1.PersistentVolumeClaim)) *MockDRVolumeInterface_setPVC_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*v1.PersistentVolumeClaim))
	})
	return _c
}

func (_c *MockDRVolumeInterface_setPVC_Call) Return() *MockDRVolumeInterface_setPVC_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockDRVolumeInterface_setPVC_Call) RunAndReturn(run func(*v1.PersistentVolumeClaim)) *MockDRVolumeInterface_setPVC_Call {
	_c.Run(run)
	return _c
}

// NewMockDRVolumeInterface creates a new instance of MockDRVolumeInterface. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockDRVolumeInterface(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockDRVolumeInterface {
	mock := &MockDRVolumeInterface{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
