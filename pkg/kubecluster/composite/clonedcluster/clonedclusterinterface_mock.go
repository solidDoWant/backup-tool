// Code generated by mockery v2.51.0. DO NOT EDIT.

package clonedcluster

import (
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	contexts "github.com/solidDoWant/backup-tool/pkg/contexts"
	clusterusercert "github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clusterusercert"

	mock "github.com/stretchr/testify/mock"

	postgres "github.com/solidDoWant/backup-tool/pkg/postgres"

	v1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
)

// MockClonedClusterInterface is an autogenerated mock type for the ClonedClusterInterface type
type MockClonedClusterInterface struct {
	mock.Mock
}

type MockClonedClusterInterface_Expecter struct {
	mock *mock.Mock
}

func (_m *MockClonedClusterInterface) EXPECT() *MockClonedClusterInterface_Expecter {
	return &MockClonedClusterInterface_Expecter{mock: &_m.Mock}
}

// Delete provides a mock function with given fields: ctx
func (_m *MockClonedClusterInterface) Delete(ctx *contexts.Context) error {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for Delete")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(*contexts.Context) error); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockClonedClusterInterface_Delete_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Delete'
type MockClonedClusterInterface_Delete_Call struct {
	*mock.Call
}

// Delete is a helper method to define mock.On call
//   - ctx *contexts.Context
func (_e *MockClonedClusterInterface_Expecter) Delete(ctx interface{}) *MockClonedClusterInterface_Delete_Call {
	return &MockClonedClusterInterface_Delete_Call{Call: _e.mock.On("Delete", ctx)}
}

func (_c *MockClonedClusterInterface_Delete_Call) Run(run func(ctx *contexts.Context)) *MockClonedClusterInterface_Delete_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*contexts.Context))
	})
	return _c
}

func (_c *MockClonedClusterInterface_Delete_Call) Return(_a0 error) *MockClonedClusterInterface_Delete_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockClonedClusterInterface_Delete_Call) RunAndReturn(run func(*contexts.Context) error) *MockClonedClusterInterface_Delete_Call {
	_c.Call.Return(run)
	return _c
}

// GetClientCACert provides a mock function with no fields
func (_m *MockClonedClusterInterface) GetClientCACert() *v1.Certificate {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetClientCACert")
	}

	var r0 *v1.Certificate
	if rf, ok := ret.Get(0).(func() *v1.Certificate); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.Certificate)
		}
	}

	return r0
}

// MockClonedClusterInterface_GetClientCACert_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetClientCACert'
type MockClonedClusterInterface_GetClientCACert_Call struct {
	*mock.Call
}

// GetClientCACert is a helper method to define mock.On call
func (_e *MockClonedClusterInterface_Expecter) GetClientCACert() *MockClonedClusterInterface_GetClientCACert_Call {
	return &MockClonedClusterInterface_GetClientCACert_Call{Call: _e.mock.On("GetClientCACert")}
}

func (_c *MockClonedClusterInterface_GetClientCACert_Call) Run(run func()) *MockClonedClusterInterface_GetClientCACert_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockClonedClusterInterface_GetClientCACert_Call) Return(_a0 *v1.Certificate) *MockClonedClusterInterface_GetClientCACert_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockClonedClusterInterface_GetClientCACert_Call) RunAndReturn(run func() *v1.Certificate) *MockClonedClusterInterface_GetClientCACert_Call {
	_c.Call.Return(run)
	return _c
}

// GetClientCAIssuer provides a mock function with no fields
func (_m *MockClonedClusterInterface) GetClientCAIssuer() *v1.Issuer {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetClientCAIssuer")
	}

	var r0 *v1.Issuer
	if rf, ok := ret.Get(0).(func() *v1.Issuer); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.Issuer)
		}
	}

	return r0
}

// MockClonedClusterInterface_GetClientCAIssuer_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetClientCAIssuer'
type MockClonedClusterInterface_GetClientCAIssuer_Call struct {
	*mock.Call
}

// GetClientCAIssuer is a helper method to define mock.On call
func (_e *MockClonedClusterInterface_Expecter) GetClientCAIssuer() *MockClonedClusterInterface_GetClientCAIssuer_Call {
	return &MockClonedClusterInterface_GetClientCAIssuer_Call{Call: _e.mock.On("GetClientCAIssuer")}
}

func (_c *MockClonedClusterInterface_GetClientCAIssuer_Call) Run(run func()) *MockClonedClusterInterface_GetClientCAIssuer_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockClonedClusterInterface_GetClientCAIssuer_Call) Return(_a0 *v1.Issuer) *MockClonedClusterInterface_GetClientCAIssuer_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockClonedClusterInterface_GetClientCAIssuer_Call) RunAndReturn(run func() *v1.Issuer) *MockClonedClusterInterface_GetClientCAIssuer_Call {
	_c.Call.Return(run)
	return _c
}

// GetCluster provides a mock function with no fields
func (_m *MockClonedClusterInterface) GetCluster() *apiv1.Cluster {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetCluster")
	}

	var r0 *apiv1.Cluster
	if rf, ok := ret.Get(0).(func() *apiv1.Cluster); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*apiv1.Cluster)
		}
	}

	return r0
}

// MockClonedClusterInterface_GetCluster_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetCluster'
type MockClonedClusterInterface_GetCluster_Call struct {
	*mock.Call
}

// GetCluster is a helper method to define mock.On call
func (_e *MockClonedClusterInterface_Expecter) GetCluster() *MockClonedClusterInterface_GetCluster_Call {
	return &MockClonedClusterInterface_GetCluster_Call{Call: _e.mock.On("GetCluster")}
}

func (_c *MockClonedClusterInterface_GetCluster_Call) Run(run func()) *MockClonedClusterInterface_GetCluster_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockClonedClusterInterface_GetCluster_Call) Return(_a0 *apiv1.Cluster) *MockClonedClusterInterface_GetCluster_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockClonedClusterInterface_GetCluster_Call) RunAndReturn(run func() *apiv1.Cluster) *MockClonedClusterInterface_GetCluster_Call {
	_c.Call.Return(run)
	return _c
}

// GetCredentials provides a mock function with given fields: servingCertMountDirectory, clientCertMountDirectory
func (_m *MockClonedClusterInterface) GetCredentials(servingCertMountDirectory string, clientCertMountDirectory string) postgres.Credentials {
	ret := _m.Called(servingCertMountDirectory, clientCertMountDirectory)

	if len(ret) == 0 {
		panic("no return value specified for GetCredentials")
	}

	var r0 postgres.Credentials
	if rf, ok := ret.Get(0).(func(string, string) postgres.Credentials); ok {
		r0 = rf(servingCertMountDirectory, clientCertMountDirectory)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(postgres.Credentials)
		}
	}

	return r0
}

// MockClonedClusterInterface_GetCredentials_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetCredentials'
type MockClonedClusterInterface_GetCredentials_Call struct {
	*mock.Call
}

// GetCredentials is a helper method to define mock.On call
//   - servingCertMountDirectory string
//   - clientCertMountDirectory string
func (_e *MockClonedClusterInterface_Expecter) GetCredentials(servingCertMountDirectory interface{}, clientCertMountDirectory interface{}) *MockClonedClusterInterface_GetCredentials_Call {
	return &MockClonedClusterInterface_GetCredentials_Call{Call: _e.mock.On("GetCredentials", servingCertMountDirectory, clientCertMountDirectory)}
}

func (_c *MockClonedClusterInterface_GetCredentials_Call) Run(run func(servingCertMountDirectory string, clientCertMountDirectory string)) *MockClonedClusterInterface_GetCredentials_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(string), args[1].(string))
	})
	return _c
}

func (_c *MockClonedClusterInterface_GetCredentials_Call) Return(_a0 postgres.Credentials) *MockClonedClusterInterface_GetCredentials_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockClonedClusterInterface_GetCredentials_Call) RunAndReturn(run func(string, string) postgres.Credentials) *MockClonedClusterInterface_GetCredentials_Call {
	_c.Call.Return(run)
	return _c
}

// GetPostgresUserCert provides a mock function with no fields
func (_m *MockClonedClusterInterface) GetPostgresUserCert() clusterusercert.ClusterUserCertInterface {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetPostgresUserCert")
	}

	var r0 clusterusercert.ClusterUserCertInterface
	if rf, ok := ret.Get(0).(func() clusterusercert.ClusterUserCertInterface); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(clusterusercert.ClusterUserCertInterface)
		}
	}

	return r0
}

// MockClonedClusterInterface_GetPostgresUserCert_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetPostgresUserCert'
type MockClonedClusterInterface_GetPostgresUserCert_Call struct {
	*mock.Call
}

// GetPostgresUserCert is a helper method to define mock.On call
func (_e *MockClonedClusterInterface_Expecter) GetPostgresUserCert() *MockClonedClusterInterface_GetPostgresUserCert_Call {
	return &MockClonedClusterInterface_GetPostgresUserCert_Call{Call: _e.mock.On("GetPostgresUserCert")}
}

func (_c *MockClonedClusterInterface_GetPostgresUserCert_Call) Run(run func()) *MockClonedClusterInterface_GetPostgresUserCert_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockClonedClusterInterface_GetPostgresUserCert_Call) Return(_a0 clusterusercert.ClusterUserCertInterface) *MockClonedClusterInterface_GetPostgresUserCert_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockClonedClusterInterface_GetPostgresUserCert_Call) RunAndReturn(run func() clusterusercert.ClusterUserCertInterface) *MockClonedClusterInterface_GetPostgresUserCert_Call {
	_c.Call.Return(run)
	return _c
}

// GetServingCert provides a mock function with no fields
func (_m *MockClonedClusterInterface) GetServingCert() *v1.Certificate {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetServingCert")
	}

	var r0 *v1.Certificate
	if rf, ok := ret.Get(0).(func() *v1.Certificate); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.Certificate)
		}
	}

	return r0
}

// MockClonedClusterInterface_GetServingCert_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetServingCert'
type MockClonedClusterInterface_GetServingCert_Call struct {
	*mock.Call
}

// GetServingCert is a helper method to define mock.On call
func (_e *MockClonedClusterInterface_Expecter) GetServingCert() *MockClonedClusterInterface_GetServingCert_Call {
	return &MockClonedClusterInterface_GetServingCert_Call{Call: _e.mock.On("GetServingCert")}
}

func (_c *MockClonedClusterInterface_GetServingCert_Call) Run(run func()) *MockClonedClusterInterface_GetServingCert_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockClonedClusterInterface_GetServingCert_Call) Return(_a0 *v1.Certificate) *MockClonedClusterInterface_GetServingCert_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockClonedClusterInterface_GetServingCert_Call) RunAndReturn(run func() *v1.Certificate) *MockClonedClusterInterface_GetServingCert_Call {
	_c.Call.Return(run)
	return _c
}

// GetStreamingReplicaUserCert provides a mock function with no fields
func (_m *MockClonedClusterInterface) GetStreamingReplicaUserCert() clusterusercert.ClusterUserCertInterface {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetStreamingReplicaUserCert")
	}

	var r0 clusterusercert.ClusterUserCertInterface
	if rf, ok := ret.Get(0).(func() clusterusercert.ClusterUserCertInterface); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(clusterusercert.ClusterUserCertInterface)
		}
	}

	return r0
}

// MockClonedClusterInterface_GetStreamingReplicaUserCert_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetStreamingReplicaUserCert'
type MockClonedClusterInterface_GetStreamingReplicaUserCert_Call struct {
	*mock.Call
}

// GetStreamingReplicaUserCert is a helper method to define mock.On call
func (_e *MockClonedClusterInterface_Expecter) GetStreamingReplicaUserCert() *MockClonedClusterInterface_GetStreamingReplicaUserCert_Call {
	return &MockClonedClusterInterface_GetStreamingReplicaUserCert_Call{Call: _e.mock.On("GetStreamingReplicaUserCert")}
}

func (_c *MockClonedClusterInterface_GetStreamingReplicaUserCert_Call) Run(run func()) *MockClonedClusterInterface_GetStreamingReplicaUserCert_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockClonedClusterInterface_GetStreamingReplicaUserCert_Call) Return(_a0 clusterusercert.ClusterUserCertInterface) *MockClonedClusterInterface_GetStreamingReplicaUserCert_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockClonedClusterInterface_GetStreamingReplicaUserCert_Call) RunAndReturn(run func() clusterusercert.ClusterUserCertInterface) *MockClonedClusterInterface_GetStreamingReplicaUserCert_Call {
	_c.Call.Return(run)
	return _c
}

// setClientCACert provides a mock function with given fields: cert
func (_m *MockClonedClusterInterface) setClientCACert(cert *v1.Certificate) {
	_m.Called(cert)
}

// MockClonedClusterInterface_setClientCACert_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'setClientCACert'
type MockClonedClusterInterface_setClientCACert_Call struct {
	*mock.Call
}

// setClientCACert is a helper method to define mock.On call
//   - cert *v1.Certificate
func (_e *MockClonedClusterInterface_Expecter) setClientCACert(cert interface{}) *MockClonedClusterInterface_setClientCACert_Call {
	return &MockClonedClusterInterface_setClientCACert_Call{Call: _e.mock.On("setClientCACert", cert)}
}

func (_c *MockClonedClusterInterface_setClientCACert_Call) Run(run func(cert *v1.Certificate)) *MockClonedClusterInterface_setClientCACert_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*v1.Certificate))
	})
	return _c
}

func (_c *MockClonedClusterInterface_setClientCACert_Call) Return() *MockClonedClusterInterface_setClientCACert_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockClonedClusterInterface_setClientCACert_Call) RunAndReturn(run func(*v1.Certificate)) *MockClonedClusterInterface_setClientCACert_Call {
	_c.Run(run)
	return _c
}

// setClientCAIssuer provides a mock function with given fields: issuer
func (_m *MockClonedClusterInterface) setClientCAIssuer(issuer *v1.Issuer) {
	_m.Called(issuer)
}

// MockClonedClusterInterface_setClientCAIssuer_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'setClientCAIssuer'
type MockClonedClusterInterface_setClientCAIssuer_Call struct {
	*mock.Call
}

// setClientCAIssuer is a helper method to define mock.On call
//   - issuer *v1.Issuer
func (_e *MockClonedClusterInterface_Expecter) setClientCAIssuer(issuer interface{}) *MockClonedClusterInterface_setClientCAIssuer_Call {
	return &MockClonedClusterInterface_setClientCAIssuer_Call{Call: _e.mock.On("setClientCAIssuer", issuer)}
}

func (_c *MockClonedClusterInterface_setClientCAIssuer_Call) Run(run func(issuer *v1.Issuer)) *MockClonedClusterInterface_setClientCAIssuer_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*v1.Issuer))
	})
	return _c
}

func (_c *MockClonedClusterInterface_setClientCAIssuer_Call) Return() *MockClonedClusterInterface_setClientCAIssuer_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockClonedClusterInterface_setClientCAIssuer_Call) RunAndReturn(run func(*v1.Issuer)) *MockClonedClusterInterface_setClientCAIssuer_Call {
	_c.Run(run)
	return _c
}

// setCluster provides a mock function with given fields: cluster
func (_m *MockClonedClusterInterface) setCluster(cluster *apiv1.Cluster) {
	_m.Called(cluster)
}

// MockClonedClusterInterface_setCluster_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'setCluster'
type MockClonedClusterInterface_setCluster_Call struct {
	*mock.Call
}

// setCluster is a helper method to define mock.On call
//   - cluster *apiv1.Cluster
func (_e *MockClonedClusterInterface_Expecter) setCluster(cluster interface{}) *MockClonedClusterInterface_setCluster_Call {
	return &MockClonedClusterInterface_setCluster_Call{Call: _e.mock.On("setCluster", cluster)}
}

func (_c *MockClonedClusterInterface_setCluster_Call) Run(run func(cluster *apiv1.Cluster)) *MockClonedClusterInterface_setCluster_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*apiv1.Cluster))
	})
	return _c
}

func (_c *MockClonedClusterInterface_setCluster_Call) Return() *MockClonedClusterInterface_setCluster_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockClonedClusterInterface_setCluster_Call) RunAndReturn(run func(*apiv1.Cluster)) *MockClonedClusterInterface_setCluster_Call {
	_c.Run(run)
	return _c
}

// setPostgresUserCert provides a mock function with given fields: cuc
func (_m *MockClonedClusterInterface) setPostgresUserCert(cuc clusterusercert.ClusterUserCertInterface) {
	_m.Called(cuc)
}

// MockClonedClusterInterface_setPostgresUserCert_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'setPostgresUserCert'
type MockClonedClusterInterface_setPostgresUserCert_Call struct {
	*mock.Call
}

// setPostgresUserCert is a helper method to define mock.On call
//   - cuc clusterusercert.ClusterUserCertInterface
func (_e *MockClonedClusterInterface_Expecter) setPostgresUserCert(cuc interface{}) *MockClonedClusterInterface_setPostgresUserCert_Call {
	return &MockClonedClusterInterface_setPostgresUserCert_Call{Call: _e.mock.On("setPostgresUserCert", cuc)}
}

func (_c *MockClonedClusterInterface_setPostgresUserCert_Call) Run(run func(cuc clusterusercert.ClusterUserCertInterface)) *MockClonedClusterInterface_setPostgresUserCert_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(clusterusercert.ClusterUserCertInterface))
	})
	return _c
}

func (_c *MockClonedClusterInterface_setPostgresUserCert_Call) Return() *MockClonedClusterInterface_setPostgresUserCert_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockClonedClusterInterface_setPostgresUserCert_Call) RunAndReturn(run func(clusterusercert.ClusterUserCertInterface)) *MockClonedClusterInterface_setPostgresUserCert_Call {
	_c.Run(run)
	return _c
}

// setServingCert provides a mock function with given fields: cert
func (_m *MockClonedClusterInterface) setServingCert(cert *v1.Certificate) {
	_m.Called(cert)
}

// MockClonedClusterInterface_setServingCert_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'setServingCert'
type MockClonedClusterInterface_setServingCert_Call struct {
	*mock.Call
}

// setServingCert is a helper method to define mock.On call
//   - cert *v1.Certificate
func (_e *MockClonedClusterInterface_Expecter) setServingCert(cert interface{}) *MockClonedClusterInterface_setServingCert_Call {
	return &MockClonedClusterInterface_setServingCert_Call{Call: _e.mock.On("setServingCert", cert)}
}

func (_c *MockClonedClusterInterface_setServingCert_Call) Run(run func(cert *v1.Certificate)) *MockClonedClusterInterface_setServingCert_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*v1.Certificate))
	})
	return _c
}

func (_c *MockClonedClusterInterface_setServingCert_Call) Return() *MockClonedClusterInterface_setServingCert_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockClonedClusterInterface_setServingCert_Call) RunAndReturn(run func(*v1.Certificate)) *MockClonedClusterInterface_setServingCert_Call {
	_c.Run(run)
	return _c
}

// setStreamingReplicaUserCert provides a mock function with given fields: cuc
func (_m *MockClonedClusterInterface) setStreamingReplicaUserCert(cuc clusterusercert.ClusterUserCertInterface) {
	_m.Called(cuc)
}

// MockClonedClusterInterface_setStreamingReplicaUserCert_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'setStreamingReplicaUserCert'
type MockClonedClusterInterface_setStreamingReplicaUserCert_Call struct {
	*mock.Call
}

// setStreamingReplicaUserCert is a helper method to define mock.On call
//   - cuc clusterusercert.ClusterUserCertInterface
func (_e *MockClonedClusterInterface_Expecter) setStreamingReplicaUserCert(cuc interface{}) *MockClonedClusterInterface_setStreamingReplicaUserCert_Call {
	return &MockClonedClusterInterface_setStreamingReplicaUserCert_Call{Call: _e.mock.On("setStreamingReplicaUserCert", cuc)}
}

func (_c *MockClonedClusterInterface_setStreamingReplicaUserCert_Call) Run(run func(cuc clusterusercert.ClusterUserCertInterface)) *MockClonedClusterInterface_setStreamingReplicaUserCert_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(clusterusercert.ClusterUserCertInterface))
	})
	return _c
}

func (_c *MockClonedClusterInterface_setStreamingReplicaUserCert_Call) Return() *MockClonedClusterInterface_setStreamingReplicaUserCert_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockClonedClusterInterface_setStreamingReplicaUserCert_Call) RunAndReturn(run func(clusterusercert.ClusterUserCertInterface)) *MockClonedClusterInterface_setStreamingReplicaUserCert_Call {
	_c.Run(run)
	return _c
}

// NewMockClonedClusterInterface creates a new instance of MockClonedClusterInterface. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockClonedClusterInterface(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockClonedClusterInterface {
	mock := &MockClonedClusterInterface{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
