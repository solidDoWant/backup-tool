// Code generated by mockery v2.51.0. DO NOT EDIT.

package disasterrecovery

import (
	contexts "github.com/solidDoWant/backup-tool/pkg/contexts"
	kubecluster "github.com/solidDoWant/backup-tool/pkg/kubecluster"

	mock "github.com/stretchr/testify/mock"

	s3 "github.com/solidDoWant/backup-tool/pkg/s3"
)

// MockS3SyncInterface is an autogenerated mock type for the S3SyncInterface type
type MockS3SyncInterface struct {
	mock.Mock
}

type MockS3SyncInterface_Expecter struct {
	mock *mock.Mock
}

func (_m *MockS3SyncInterface) EXPECT() *MockS3SyncInterface_Expecter {
	return &MockS3SyncInterface_Expecter{mock: &_m.Mock}
}

// Configure provides a mock function with given fields: kubeClusterClient, namespace, drVolName, dirName, s3Path, eventName, credentials, opts
func (_m *MockS3SyncInterface) Configure(kubeClusterClient kubecluster.ClientInterface, namespace string, drVolName string, dirName string, s3Path string, eventName string, credentials s3.CredentialsInterface, opts s3SyncOpts) {
	_m.Called(kubeClusterClient, namespace, drVolName, dirName, s3Path, eventName, credentials, opts)
}

// MockS3SyncInterface_Configure_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Configure'
type MockS3SyncInterface_Configure_Call struct {
	*mock.Call
}

// Configure is a helper method to define mock.On call
//   - kubeClusterClient kubecluster.ClientInterface
//   - namespace string
//   - drVolName string
//   - dirName string
//   - s3Path string
//   - eventName string
//   - credentials s3.CredentialsInterface
//   - opts s3SyncOpts
func (_e *MockS3SyncInterface_Expecter) Configure(kubeClusterClient interface{}, namespace interface{}, drVolName interface{}, dirName interface{}, s3Path interface{}, eventName interface{}, credentials interface{}, opts interface{}) *MockS3SyncInterface_Configure_Call {
	return &MockS3SyncInterface_Configure_Call{Call: _e.mock.On("Configure", kubeClusterClient, namespace, drVolName, dirName, s3Path, eventName, credentials, opts)}
}

func (_c *MockS3SyncInterface_Configure_Call) Run(run func(kubeClusterClient kubecluster.ClientInterface, namespace string, drVolName string, dirName string, s3Path string, eventName string, credentials s3.CredentialsInterface, opts s3SyncOpts)) *MockS3SyncInterface_Configure_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(kubecluster.ClientInterface), args[1].(string), args[2].(string), args[3].(string), args[4].(string), args[5].(string), args[6].(s3.CredentialsInterface), args[7].(s3SyncOpts))
	})
	return _c
}

func (_c *MockS3SyncInterface_Configure_Call) Return() *MockS3SyncInterface_Configure_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockS3SyncInterface_Configure_Call) RunAndReturn(run func(kubecluster.ClientInterface, string, string, string, string, string, s3.CredentialsInterface, s3SyncOpts)) *MockS3SyncInterface_Configure_Call {
	_c.Run(run)
	return _c
}

// Sync provides a mock function with given fields: ctx
func (_m *MockS3SyncInterface) Sync(ctx *contexts.Context) error {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for Sync")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(*contexts.Context) error); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockS3SyncInterface_Sync_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Sync'
type MockS3SyncInterface_Sync_Call struct {
	*mock.Call
}

// Sync is a helper method to define mock.On call
//   - ctx *contexts.Context
func (_e *MockS3SyncInterface_Expecter) Sync(ctx interface{}) *MockS3SyncInterface_Sync_Call {
	return &MockS3SyncInterface_Sync_Call{Call: _e.mock.On("Sync", ctx)}
}

func (_c *MockS3SyncInterface_Sync_Call) Run(run func(ctx *contexts.Context)) *MockS3SyncInterface_Sync_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*contexts.Context))
	})
	return _c
}

func (_c *MockS3SyncInterface_Sync_Call) Return(_a0 error) *MockS3SyncInterface_Sync_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockS3SyncInterface_Sync_Call) RunAndReturn(run func(*contexts.Context) error) *MockS3SyncInterface_Sync_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockS3SyncInterface creates a new instance of MockS3SyncInterface. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockS3SyncInterface(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockS3SyncInterface {
	mock := &MockS3SyncInterface{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
