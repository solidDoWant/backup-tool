// Code generated by mockery v2.51.0. DO NOT EDIT.

package cnpg

import (
	contexts "github.com/solidDoWant/backup-tool/pkg/contexts"
	mock "github.com/stretchr/testify/mock"

	resource "k8s.io/apimachinery/pkg/api/resource"

	v1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
)

// MockClientInterface is an autogenerated mock type for the ClientInterface type
type MockClientInterface struct {
	mock.Mock
}

type MockClientInterface_Expecter struct {
	mock *mock.Mock
}

func (_m *MockClientInterface) EXPECT() *MockClientInterface_Expecter {
	return &MockClientInterface_Expecter{mock: &_m.Mock}
}

// CreateBackup provides a mock function with given fields: ctx, namespace, backupName, clusterName, opts
func (_m *MockClientInterface) CreateBackup(ctx *contexts.Context, namespace string, backupName string, clusterName string, opts CreateBackupOptions) (*v1.Backup, error) {
	ret := _m.Called(ctx, namespace, backupName, clusterName, opts)

	if len(ret) == 0 {
		panic("no return value specified for CreateBackup")
	}

	var r0 *v1.Backup
	var r1 error
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, string, CreateBackupOptions) (*v1.Backup, error)); ok {
		return rf(ctx, namespace, backupName, clusterName, opts)
	}
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, string, CreateBackupOptions) *v1.Backup); ok {
		r0 = rf(ctx, namespace, backupName, clusterName, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.Backup)
		}
	}

	if rf, ok := ret.Get(1).(func(*contexts.Context, string, string, string, CreateBackupOptions) error); ok {
		r1 = rf(ctx, namespace, backupName, clusterName, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockClientInterface_CreateBackup_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'CreateBackup'
type MockClientInterface_CreateBackup_Call struct {
	*mock.Call
}

// CreateBackup is a helper method to define mock.On call
//   - ctx *contexts.Context
//   - namespace string
//   - backupName string
//   - clusterName string
//   - opts CreateBackupOptions
func (_e *MockClientInterface_Expecter) CreateBackup(ctx interface{}, namespace interface{}, backupName interface{}, clusterName interface{}, opts interface{}) *MockClientInterface_CreateBackup_Call {
	return &MockClientInterface_CreateBackup_Call{Call: _e.mock.On("CreateBackup", ctx, namespace, backupName, clusterName, opts)}
}

func (_c *MockClientInterface_CreateBackup_Call) Run(run func(ctx *contexts.Context, namespace string, backupName string, clusterName string, opts CreateBackupOptions)) *MockClientInterface_CreateBackup_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*contexts.Context), args[1].(string), args[2].(string), args[3].(string), args[4].(CreateBackupOptions))
	})
	return _c
}

func (_c *MockClientInterface_CreateBackup_Call) Return(_a0 *v1.Backup, _a1 error) *MockClientInterface_CreateBackup_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockClientInterface_CreateBackup_Call) RunAndReturn(run func(*contexts.Context, string, string, string, CreateBackupOptions) (*v1.Backup, error)) *MockClientInterface_CreateBackup_Call {
	_c.Call.Return(run)
	return _c
}

// CreateCluster provides a mock function with given fields: ctx, namespace, clusterName, volumeSize, servingCertificateSecretName, clientCASecretName, replicationUserCertName, opts
func (_m *MockClientInterface) CreateCluster(ctx *contexts.Context, namespace string, clusterName string, volumeSize resource.Quantity, servingCertificateSecretName string, clientCASecretName string, replicationUserCertName string, opts CreateClusterOptions) (*v1.Cluster, error) {
	ret := _m.Called(ctx, namespace, clusterName, volumeSize, servingCertificateSecretName, clientCASecretName, replicationUserCertName, opts)

	if len(ret) == 0 {
		panic("no return value specified for CreateCluster")
	}

	var r0 *v1.Cluster
	var r1 error
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, resource.Quantity, string, string, string, CreateClusterOptions) (*v1.Cluster, error)); ok {
		return rf(ctx, namespace, clusterName, volumeSize, servingCertificateSecretName, clientCASecretName, replicationUserCertName, opts)
	}
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, resource.Quantity, string, string, string, CreateClusterOptions) *v1.Cluster); ok {
		r0 = rf(ctx, namespace, clusterName, volumeSize, servingCertificateSecretName, clientCASecretName, replicationUserCertName, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.Cluster)
		}
	}

	if rf, ok := ret.Get(1).(func(*contexts.Context, string, string, resource.Quantity, string, string, string, CreateClusterOptions) error); ok {
		r1 = rf(ctx, namespace, clusterName, volumeSize, servingCertificateSecretName, clientCASecretName, replicationUserCertName, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockClientInterface_CreateCluster_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'CreateCluster'
type MockClientInterface_CreateCluster_Call struct {
	*mock.Call
}

// CreateCluster is a helper method to define mock.On call
//   - ctx *contexts.Context
//   - namespace string
//   - clusterName string
//   - volumeSize resource.Quantity
//   - servingCertificateSecretName string
//   - clientCASecretName string
//   - replicationUserCertName string
//   - opts CreateClusterOptions
func (_e *MockClientInterface_Expecter) CreateCluster(ctx interface{}, namespace interface{}, clusterName interface{}, volumeSize interface{}, servingCertificateSecretName interface{}, clientCASecretName interface{}, replicationUserCertName interface{}, opts interface{}) *MockClientInterface_CreateCluster_Call {
	return &MockClientInterface_CreateCluster_Call{Call: _e.mock.On("CreateCluster", ctx, namespace, clusterName, volumeSize, servingCertificateSecretName, clientCASecretName, replicationUserCertName, opts)}
}

func (_c *MockClientInterface_CreateCluster_Call) Run(run func(ctx *contexts.Context, namespace string, clusterName string, volumeSize resource.Quantity, servingCertificateSecretName string, clientCASecretName string, replicationUserCertName string, opts CreateClusterOptions)) *MockClientInterface_CreateCluster_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*contexts.Context), args[1].(string), args[2].(string), args[3].(resource.Quantity), args[4].(string), args[5].(string), args[6].(string), args[7].(CreateClusterOptions))
	})
	return _c
}

func (_c *MockClientInterface_CreateCluster_Call) Return(_a0 *v1.Cluster, _a1 error) *MockClientInterface_CreateCluster_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockClientInterface_CreateCluster_Call) RunAndReturn(run func(*contexts.Context, string, string, resource.Quantity, string, string, string, CreateClusterOptions) (*v1.Cluster, error)) *MockClientInterface_CreateCluster_Call {
	_c.Call.Return(run)
	return _c
}

// DeleteBackup provides a mock function with given fields: ctx, namespace, name
func (_m *MockClientInterface) DeleteBackup(ctx *contexts.Context, namespace string, name string) error {
	ret := _m.Called(ctx, namespace, name)

	if len(ret) == 0 {
		panic("no return value specified for DeleteBackup")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string) error); ok {
		r0 = rf(ctx, namespace, name)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockClientInterface_DeleteBackup_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'DeleteBackup'
type MockClientInterface_DeleteBackup_Call struct {
	*mock.Call
}

// DeleteBackup is a helper method to define mock.On call
//   - ctx *contexts.Context
//   - namespace string
//   - name string
func (_e *MockClientInterface_Expecter) DeleteBackup(ctx interface{}, namespace interface{}, name interface{}) *MockClientInterface_DeleteBackup_Call {
	return &MockClientInterface_DeleteBackup_Call{Call: _e.mock.On("DeleteBackup", ctx, namespace, name)}
}

func (_c *MockClientInterface_DeleteBackup_Call) Run(run func(ctx *contexts.Context, namespace string, name string)) *MockClientInterface_DeleteBackup_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*contexts.Context), args[1].(string), args[2].(string))
	})
	return _c
}

func (_c *MockClientInterface_DeleteBackup_Call) Return(_a0 error) *MockClientInterface_DeleteBackup_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockClientInterface_DeleteBackup_Call) RunAndReturn(run func(*contexts.Context, string, string) error) *MockClientInterface_DeleteBackup_Call {
	_c.Call.Return(run)
	return _c
}

// DeleteCluster provides a mock function with given fields: ctx, namespace, name
func (_m *MockClientInterface) DeleteCluster(ctx *contexts.Context, namespace string, name string) error {
	ret := _m.Called(ctx, namespace, name)

	if len(ret) == 0 {
		panic("no return value specified for DeleteCluster")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string) error); ok {
		r0 = rf(ctx, namespace, name)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockClientInterface_DeleteCluster_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'DeleteCluster'
type MockClientInterface_DeleteCluster_Call struct {
	*mock.Call
}

// DeleteCluster is a helper method to define mock.On call
//   - ctx *contexts.Context
//   - namespace string
//   - name string
func (_e *MockClientInterface_Expecter) DeleteCluster(ctx interface{}, namespace interface{}, name interface{}) *MockClientInterface_DeleteCluster_Call {
	return &MockClientInterface_DeleteCluster_Call{Call: _e.mock.On("DeleteCluster", ctx, namespace, name)}
}

func (_c *MockClientInterface_DeleteCluster_Call) Run(run func(ctx *contexts.Context, namespace string, name string)) *MockClientInterface_DeleteCluster_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*contexts.Context), args[1].(string), args[2].(string))
	})
	return _c
}

func (_c *MockClientInterface_DeleteCluster_Call) Return(_a0 error) *MockClientInterface_DeleteCluster_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockClientInterface_DeleteCluster_Call) RunAndReturn(run func(*contexts.Context, string, string) error) *MockClientInterface_DeleteCluster_Call {
	_c.Call.Return(run)
	return _c
}

// GetCluster provides a mock function with given fields: ctx, namespace, name
func (_m *MockClientInterface) GetCluster(ctx *contexts.Context, namespace string, name string) (*v1.Cluster, error) {
	ret := _m.Called(ctx, namespace, name)

	if len(ret) == 0 {
		panic("no return value specified for GetCluster")
	}

	var r0 *v1.Cluster
	var r1 error
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string) (*v1.Cluster, error)); ok {
		return rf(ctx, namespace, name)
	}
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string) *v1.Cluster); ok {
		r0 = rf(ctx, namespace, name)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.Cluster)
		}
	}

	if rf, ok := ret.Get(1).(func(*contexts.Context, string, string) error); ok {
		r1 = rf(ctx, namespace, name)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockClientInterface_GetCluster_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetCluster'
type MockClientInterface_GetCluster_Call struct {
	*mock.Call
}

// GetCluster is a helper method to define mock.On call
//   - ctx *contexts.Context
//   - namespace string
//   - name string
func (_e *MockClientInterface_Expecter) GetCluster(ctx interface{}, namespace interface{}, name interface{}) *MockClientInterface_GetCluster_Call {
	return &MockClientInterface_GetCluster_Call{Call: _e.mock.On("GetCluster", ctx, namespace, name)}
}

func (_c *MockClientInterface_GetCluster_Call) Run(run func(ctx *contexts.Context, namespace string, name string)) *MockClientInterface_GetCluster_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*contexts.Context), args[1].(string), args[2].(string))
	})
	return _c
}

func (_c *MockClientInterface_GetCluster_Call) Return(_a0 *v1.Cluster, _a1 error) *MockClientInterface_GetCluster_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockClientInterface_GetCluster_Call) RunAndReturn(run func(*contexts.Context, string, string) (*v1.Cluster, error)) *MockClientInterface_GetCluster_Call {
	_c.Call.Return(run)
	return _c
}

// SetCommonLabels provides a mock function with given fields: labels
func (_m *MockClientInterface) SetCommonLabels(labels map[string]string) {
	_m.Called(labels)
}

// MockClientInterface_SetCommonLabels_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'SetCommonLabels'
type MockClientInterface_SetCommonLabels_Call struct {
	*mock.Call
}

// SetCommonLabels is a helper method to define mock.On call
//   - labels map[string]string
func (_e *MockClientInterface_Expecter) SetCommonLabels(labels interface{}) *MockClientInterface_SetCommonLabels_Call {
	return &MockClientInterface_SetCommonLabels_Call{Call: _e.mock.On("SetCommonLabels", labels)}
}

func (_c *MockClientInterface_SetCommonLabels_Call) Run(run func(labels map[string]string)) *MockClientInterface_SetCommonLabels_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(map[string]string))
	})
	return _c
}

func (_c *MockClientInterface_SetCommonLabels_Call) Return() *MockClientInterface_SetCommonLabels_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockClientInterface_SetCommonLabels_Call) RunAndReturn(run func(map[string]string)) *MockClientInterface_SetCommonLabels_Call {
	_c.Run(run)
	return _c
}

// WaitForReadyBackup provides a mock function with given fields: ctx, namespace, name, opts
func (_m *MockClientInterface) WaitForReadyBackup(ctx *contexts.Context, namespace string, name string, opts WaitForReadyBackupOpts) (*v1.Backup, error) {
	ret := _m.Called(ctx, namespace, name, opts)

	if len(ret) == 0 {
		panic("no return value specified for WaitForReadyBackup")
	}

	var r0 *v1.Backup
	var r1 error
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, WaitForReadyBackupOpts) (*v1.Backup, error)); ok {
		return rf(ctx, namespace, name, opts)
	}
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, WaitForReadyBackupOpts) *v1.Backup); ok {
		r0 = rf(ctx, namespace, name, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.Backup)
		}
	}

	if rf, ok := ret.Get(1).(func(*contexts.Context, string, string, WaitForReadyBackupOpts) error); ok {
		r1 = rf(ctx, namespace, name, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockClientInterface_WaitForReadyBackup_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'WaitForReadyBackup'
type MockClientInterface_WaitForReadyBackup_Call struct {
	*mock.Call
}

// WaitForReadyBackup is a helper method to define mock.On call
//   - ctx *contexts.Context
//   - namespace string
//   - name string
//   - opts WaitForReadyBackupOpts
func (_e *MockClientInterface_Expecter) WaitForReadyBackup(ctx interface{}, namespace interface{}, name interface{}, opts interface{}) *MockClientInterface_WaitForReadyBackup_Call {
	return &MockClientInterface_WaitForReadyBackup_Call{Call: _e.mock.On("WaitForReadyBackup", ctx, namespace, name, opts)}
}

func (_c *MockClientInterface_WaitForReadyBackup_Call) Run(run func(ctx *contexts.Context, namespace string, name string, opts WaitForReadyBackupOpts)) *MockClientInterface_WaitForReadyBackup_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*contexts.Context), args[1].(string), args[2].(string), args[3].(WaitForReadyBackupOpts))
	})
	return _c
}

func (_c *MockClientInterface_WaitForReadyBackup_Call) Return(_a0 *v1.Backup, _a1 error) *MockClientInterface_WaitForReadyBackup_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockClientInterface_WaitForReadyBackup_Call) RunAndReturn(run func(*contexts.Context, string, string, WaitForReadyBackupOpts) (*v1.Backup, error)) *MockClientInterface_WaitForReadyBackup_Call {
	_c.Call.Return(run)
	return _c
}

// WaitForReadyCluster provides a mock function with given fields: ctx, namespace, name, opts
func (_m *MockClientInterface) WaitForReadyCluster(ctx *contexts.Context, namespace string, name string, opts WaitForReadyClusterOpts) (*v1.Cluster, error) {
	ret := _m.Called(ctx, namespace, name, opts)

	if len(ret) == 0 {
		panic("no return value specified for WaitForReadyCluster")
	}

	var r0 *v1.Cluster
	var r1 error
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, WaitForReadyClusterOpts) (*v1.Cluster, error)); ok {
		return rf(ctx, namespace, name, opts)
	}
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, WaitForReadyClusterOpts) *v1.Cluster); ok {
		r0 = rf(ctx, namespace, name, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.Cluster)
		}
	}

	if rf, ok := ret.Get(1).(func(*contexts.Context, string, string, WaitForReadyClusterOpts) error); ok {
		r1 = rf(ctx, namespace, name, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockClientInterface_WaitForReadyCluster_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'WaitForReadyCluster'
type MockClientInterface_WaitForReadyCluster_Call struct {
	*mock.Call
}

// WaitForReadyCluster is a helper method to define mock.On call
//   - ctx *contexts.Context
//   - namespace string
//   - name string
//   - opts WaitForReadyClusterOpts
func (_e *MockClientInterface_Expecter) WaitForReadyCluster(ctx interface{}, namespace interface{}, name interface{}, opts interface{}) *MockClientInterface_WaitForReadyCluster_Call {
	return &MockClientInterface_WaitForReadyCluster_Call{Call: _e.mock.On("WaitForReadyCluster", ctx, namespace, name, opts)}
}

func (_c *MockClientInterface_WaitForReadyCluster_Call) Run(run func(ctx *contexts.Context, namespace string, name string, opts WaitForReadyClusterOpts)) *MockClientInterface_WaitForReadyCluster_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*contexts.Context), args[1].(string), args[2].(string), args[3].(WaitForReadyClusterOpts))
	})
	return _c
}

func (_c *MockClientInterface_WaitForReadyCluster_Call) Return(_a0 *v1.Cluster, _a1 error) *MockClientInterface_WaitForReadyCluster_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockClientInterface_WaitForReadyCluster_Call) RunAndReturn(run func(*contexts.Context, string, string, WaitForReadyClusterOpts) (*v1.Cluster, error)) *MockClientInterface_WaitForReadyCluster_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockClientInterface creates a new instance of MockClientInterface. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockClientInterface(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockClientInterface {
	mock := &MockClientInterface{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
