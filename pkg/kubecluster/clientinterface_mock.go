// Code generated by mockery v2.51.0. DO NOT EDIT.

package kubecluster

import (
	backuptoolinstance "github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	approverpolicy "github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/approverpolicy"

	certmanager "github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"

	clonedcluster "github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonedcluster"

	clonepvc "github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonepvc"

	clusterusercert "github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clusterusercert"

	cnpg "github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"

	contexts "github.com/solidDoWant/backup-tool/pkg/contexts"

	core "github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"

	createcrpforcertificate "github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/createcrpforcertificate"

	drvolume "github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/drvolume"

	externalsnapshotter "github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"

	mock "github.com/stretchr/testify/mock"

	resource "k8s.io/apimachinery/pkg/api/resource"

	v1 "k8s.io/api/core/v1"

	v1alpha1 "github.com/cert-manager/approver-policy/pkg/apis/policy/v1alpha1"
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

// AP provides a mock function with no fields
func (_m *MockClientInterface) AP() approverpolicy.ClientInterface {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for AP")
	}

	var r0 approverpolicy.ClientInterface
	if rf, ok := ret.Get(0).(func() approverpolicy.ClientInterface); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(approverpolicy.ClientInterface)
		}
	}

	return r0
}

// MockClientInterface_AP_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'AP'
type MockClientInterface_AP_Call struct {
	*mock.Call
}

// AP is a helper method to define mock.On call
func (_e *MockClientInterface_Expecter) AP() *MockClientInterface_AP_Call {
	return &MockClientInterface_AP_Call{Call: _e.mock.On("AP")}
}

func (_c *MockClientInterface_AP_Call) Run(run func()) *MockClientInterface_AP_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockClientInterface_AP_Call) Return(_a0 approverpolicy.ClientInterface) *MockClientInterface_AP_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockClientInterface_AP_Call) RunAndReturn(run func() approverpolicy.ClientInterface) *MockClientInterface_AP_Call {
	_c.Call.Return(run)
	return _c
}

// CM provides a mock function with no fields
func (_m *MockClientInterface) CM() certmanager.ClientInterface {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for CM")
	}

	var r0 certmanager.ClientInterface
	if rf, ok := ret.Get(0).(func() certmanager.ClientInterface); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(certmanager.ClientInterface)
		}
	}

	return r0
}

// MockClientInterface_CM_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'CM'
type MockClientInterface_CM_Call struct {
	*mock.Call
}

// CM is a helper method to define mock.On call
func (_e *MockClientInterface_Expecter) CM() *MockClientInterface_CM_Call {
	return &MockClientInterface_CM_Call{Call: _e.mock.On("CM")}
}

func (_c *MockClientInterface_CM_Call) Run(run func()) *MockClientInterface_CM_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockClientInterface_CM_Call) Return(_a0 certmanager.ClientInterface) *MockClientInterface_CM_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockClientInterface_CM_Call) RunAndReturn(run func() certmanager.ClientInterface) *MockClientInterface_CM_Call {
	_c.Call.Return(run)
	return _c
}

// CNPG provides a mock function with no fields
func (_m *MockClientInterface) CNPG() cnpg.ClientInterface {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for CNPG")
	}

	var r0 cnpg.ClientInterface
	if rf, ok := ret.Get(0).(func() cnpg.ClientInterface); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(cnpg.ClientInterface)
		}
	}

	return r0
}

// MockClientInterface_CNPG_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'CNPG'
type MockClientInterface_CNPG_Call struct {
	*mock.Call
}

// CNPG is a helper method to define mock.On call
func (_e *MockClientInterface_Expecter) CNPG() *MockClientInterface_CNPG_Call {
	return &MockClientInterface_CNPG_Call{Call: _e.mock.On("CNPG")}
}

func (_c *MockClientInterface_CNPG_Call) Run(run func()) *MockClientInterface_CNPG_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockClientInterface_CNPG_Call) Return(_a0 cnpg.ClientInterface) *MockClientInterface_CNPG_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockClientInterface_CNPG_Call) RunAndReturn(run func() cnpg.ClientInterface) *MockClientInterface_CNPG_Call {
	_c.Call.Return(run)
	return _c
}

// CloneCluster provides a mock function with given fields: ctx, namespace, existingClusterName, newClusterName, servingCertIssuerName, clientCertIssuerName, opts
func (_m *MockClientInterface) CloneCluster(ctx *contexts.Context, namespace string, existingClusterName string, newClusterName string, servingCertIssuerName string, clientCertIssuerName string, opts clonedcluster.CloneClusterOptions) (clonedcluster.ClonedClusterInterface, error) {
	ret := _m.Called(ctx, namespace, existingClusterName, newClusterName, servingCertIssuerName, clientCertIssuerName, opts)

	if len(ret) == 0 {
		panic("no return value specified for CloneCluster")
	}

	var r0 clonedcluster.ClonedClusterInterface
	var r1 error
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, string, string, string, clonedcluster.CloneClusterOptions) (clonedcluster.ClonedClusterInterface, error)); ok {
		return rf(ctx, namespace, existingClusterName, newClusterName, servingCertIssuerName, clientCertIssuerName, opts)
	}
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, string, string, string, clonedcluster.CloneClusterOptions) clonedcluster.ClonedClusterInterface); ok {
		r0 = rf(ctx, namespace, existingClusterName, newClusterName, servingCertIssuerName, clientCertIssuerName, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(clonedcluster.ClonedClusterInterface)
		}
	}

	if rf, ok := ret.Get(1).(func(*contexts.Context, string, string, string, string, string, clonedcluster.CloneClusterOptions) error); ok {
		r1 = rf(ctx, namespace, existingClusterName, newClusterName, servingCertIssuerName, clientCertIssuerName, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockClientInterface_CloneCluster_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'CloneCluster'
type MockClientInterface_CloneCluster_Call struct {
	*mock.Call
}

// CloneCluster is a helper method to define mock.On call
//   - ctx *contexts.Context
//   - namespace string
//   - existingClusterName string
//   - newClusterName string
//   - servingCertIssuerName string
//   - clientCertIssuerName string
//   - opts clonedcluster.CloneClusterOptions
func (_e *MockClientInterface_Expecter) CloneCluster(ctx interface{}, namespace interface{}, existingClusterName interface{}, newClusterName interface{}, servingCertIssuerName interface{}, clientCertIssuerName interface{}, opts interface{}) *MockClientInterface_CloneCluster_Call {
	return &MockClientInterface_CloneCluster_Call{Call: _e.mock.On("CloneCluster", ctx, namespace, existingClusterName, newClusterName, servingCertIssuerName, clientCertIssuerName, opts)}
}

func (_c *MockClientInterface_CloneCluster_Call) Run(run func(ctx *contexts.Context, namespace string, existingClusterName string, newClusterName string, servingCertIssuerName string, clientCertIssuerName string, opts clonedcluster.CloneClusterOptions)) *MockClientInterface_CloneCluster_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*contexts.Context), args[1].(string), args[2].(string), args[3].(string), args[4].(string), args[5].(string), args[6].(clonedcluster.CloneClusterOptions))
	})
	return _c
}

func (_c *MockClientInterface_CloneCluster_Call) Return(cluster clonedcluster.ClonedClusterInterface, err error) *MockClientInterface_CloneCluster_Call {
	_c.Call.Return(cluster, err)
	return _c
}

func (_c *MockClientInterface_CloneCluster_Call) RunAndReturn(run func(*contexts.Context, string, string, string, string, string, clonedcluster.CloneClusterOptions) (clonedcluster.ClonedClusterInterface, error)) *MockClientInterface_CloneCluster_Call {
	_c.Call.Return(run)
	return _c
}

// ClonePVC provides a mock function with given fields: ctx, namespace, pvcName, opts
func (_m *MockClientInterface) ClonePVC(ctx *contexts.Context, namespace string, pvcName string, opts clonepvc.ClonePVCOptions) (*v1.PersistentVolumeClaim, error) {
	ret := _m.Called(ctx, namespace, pvcName, opts)

	if len(ret) == 0 {
		panic("no return value specified for ClonePVC")
	}

	var r0 *v1.PersistentVolumeClaim
	var r1 error
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, clonepvc.ClonePVCOptions) (*v1.PersistentVolumeClaim, error)); ok {
		return rf(ctx, namespace, pvcName, opts)
	}
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, clonepvc.ClonePVCOptions) *v1.PersistentVolumeClaim); ok {
		r0 = rf(ctx, namespace, pvcName, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.PersistentVolumeClaim)
		}
	}

	if rf, ok := ret.Get(1).(func(*contexts.Context, string, string, clonepvc.ClonePVCOptions) error); ok {
		r1 = rf(ctx, namespace, pvcName, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockClientInterface_ClonePVC_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ClonePVC'
type MockClientInterface_ClonePVC_Call struct {
	*mock.Call
}

// ClonePVC is a helper method to define mock.On call
//   - ctx *contexts.Context
//   - namespace string
//   - pvcName string
//   - opts clonepvc.ClonePVCOptions
func (_e *MockClientInterface_Expecter) ClonePVC(ctx interface{}, namespace interface{}, pvcName interface{}, opts interface{}) *MockClientInterface_ClonePVC_Call {
	return &MockClientInterface_ClonePVC_Call{Call: _e.mock.On("ClonePVC", ctx, namespace, pvcName, opts)}
}

func (_c *MockClientInterface_ClonePVC_Call) Run(run func(ctx *contexts.Context, namespace string, pvcName string, opts clonepvc.ClonePVCOptions)) *MockClientInterface_ClonePVC_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*contexts.Context), args[1].(string), args[2].(string), args[3].(clonepvc.ClonePVCOptions))
	})
	return _c
}

func (_c *MockClientInterface_ClonePVC_Call) Return(clonedPvc *v1.PersistentVolumeClaim, err error) *MockClientInterface_ClonePVC_Call {
	_c.Call.Return(clonedPvc, err)
	return _c
}

func (_c *MockClientInterface_ClonePVC_Call) RunAndReturn(run func(*contexts.Context, string, string, clonepvc.ClonePVCOptions) (*v1.PersistentVolumeClaim, error)) *MockClientInterface_ClonePVC_Call {
	_c.Call.Return(run)
	return _c
}

// Core provides a mock function with no fields
func (_m *MockClientInterface) Core() core.ClientInterface {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Core")
	}

	var r0 core.ClientInterface
	if rf, ok := ret.Get(0).(func() core.ClientInterface); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(core.ClientInterface)
		}
	}

	return r0
}

// MockClientInterface_Core_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Core'
type MockClientInterface_Core_Call struct {
	*mock.Call
}

// Core is a helper method to define mock.On call
func (_e *MockClientInterface_Expecter) Core() *MockClientInterface_Core_Call {
	return &MockClientInterface_Core_Call{Call: _e.mock.On("Core")}
}

func (_c *MockClientInterface_Core_Call) Run(run func()) *MockClientInterface_Core_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockClientInterface_Core_Call) Return(_a0 core.ClientInterface) *MockClientInterface_Core_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockClientInterface_Core_Call) RunAndReturn(run func() core.ClientInterface) *MockClientInterface_Core_Call {
	_c.Call.Return(run)
	return _c
}

// CreateBackupToolInstance provides a mock function with given fields: ctx, namespace, instance, opts
func (_m *MockClientInterface) CreateBackupToolInstance(ctx *contexts.Context, namespace string, instance string, opts backuptoolinstance.CreateBackupToolInstanceOptions) (backuptoolinstance.BackupToolInstanceInterface, error) {
	ret := _m.Called(ctx, namespace, instance, opts)

	if len(ret) == 0 {
		panic("no return value specified for CreateBackupToolInstance")
	}

	var r0 backuptoolinstance.BackupToolInstanceInterface
	var r1 error
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, backuptoolinstance.CreateBackupToolInstanceOptions) (backuptoolinstance.BackupToolInstanceInterface, error)); ok {
		return rf(ctx, namespace, instance, opts)
	}
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, backuptoolinstance.CreateBackupToolInstanceOptions) backuptoolinstance.BackupToolInstanceInterface); ok {
		r0 = rf(ctx, namespace, instance, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(backuptoolinstance.BackupToolInstanceInterface)
		}
	}

	if rf, ok := ret.Get(1).(func(*contexts.Context, string, string, backuptoolinstance.CreateBackupToolInstanceOptions) error); ok {
		r1 = rf(ctx, namespace, instance, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockClientInterface_CreateBackupToolInstance_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'CreateBackupToolInstance'
type MockClientInterface_CreateBackupToolInstance_Call struct {
	*mock.Call
}

// CreateBackupToolInstance is a helper method to define mock.On call
//   - ctx *contexts.Context
//   - namespace string
//   - instance string
//   - opts backuptoolinstance.CreateBackupToolInstanceOptions
func (_e *MockClientInterface_Expecter) CreateBackupToolInstance(ctx interface{}, namespace interface{}, instance interface{}, opts interface{}) *MockClientInterface_CreateBackupToolInstance_Call {
	return &MockClientInterface_CreateBackupToolInstance_Call{Call: _e.mock.On("CreateBackupToolInstance", ctx, namespace, instance, opts)}
}

func (_c *MockClientInterface_CreateBackupToolInstance_Call) Run(run func(ctx *contexts.Context, namespace string, instance string, opts backuptoolinstance.CreateBackupToolInstanceOptions)) *MockClientInterface_CreateBackupToolInstance_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*contexts.Context), args[1].(string), args[2].(string), args[3].(backuptoolinstance.CreateBackupToolInstanceOptions))
	})
	return _c
}

func (_c *MockClientInterface_CreateBackupToolInstance_Call) Return(btInstance backuptoolinstance.BackupToolInstanceInterface, err error) *MockClientInterface_CreateBackupToolInstance_Call {
	_c.Call.Return(btInstance, err)
	return _c
}

func (_c *MockClientInterface_CreateBackupToolInstance_Call) RunAndReturn(run func(*contexts.Context, string, string, backuptoolinstance.CreateBackupToolInstanceOptions) (backuptoolinstance.BackupToolInstanceInterface, error)) *MockClientInterface_CreateBackupToolInstance_Call {
	_c.Call.Return(run)
	return _c
}

// CreateCRPForCertificate provides a mock function with given fields: ctx, cert, opts
func (_m *MockClientInterface) CreateCRPForCertificate(ctx *contexts.Context, cert *certmanagerv1.Certificate, opts createcrpforcertificate.CreateCRPForCertificateOpts) (*v1alpha1.CertificateRequestPolicy, error) {
	ret := _m.Called(ctx, cert, opts)

	if len(ret) == 0 {
		panic("no return value specified for CreateCRPForCertificate")
	}

	var r0 *v1alpha1.CertificateRequestPolicy
	var r1 error
	if rf, ok := ret.Get(0).(func(*contexts.Context, *certmanagerv1.Certificate, createcrpforcertificate.CreateCRPForCertificateOpts) (*v1alpha1.CertificateRequestPolicy, error)); ok {
		return rf(ctx, cert, opts)
	}
	if rf, ok := ret.Get(0).(func(*contexts.Context, *certmanagerv1.Certificate, createcrpforcertificate.CreateCRPForCertificateOpts) *v1alpha1.CertificateRequestPolicy); ok {
		r0 = rf(ctx, cert, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1alpha1.CertificateRequestPolicy)
		}
	}

	if rf, ok := ret.Get(1).(func(*contexts.Context, *certmanagerv1.Certificate, createcrpforcertificate.CreateCRPForCertificateOpts) error); ok {
		r1 = rf(ctx, cert, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockClientInterface_CreateCRPForCertificate_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'CreateCRPForCertificate'
type MockClientInterface_CreateCRPForCertificate_Call struct {
	*mock.Call
}

// CreateCRPForCertificate is a helper method to define mock.On call
//   - ctx *contexts.Context
//   - cert *certmanagerv1.Certificate
//   - opts createcrpforcertificate.CreateCRPForCertificateOpts
func (_e *MockClientInterface_Expecter) CreateCRPForCertificate(ctx interface{}, cert interface{}, opts interface{}) *MockClientInterface_CreateCRPForCertificate_Call {
	return &MockClientInterface_CreateCRPForCertificate_Call{Call: _e.mock.On("CreateCRPForCertificate", ctx, cert, opts)}
}

func (_c *MockClientInterface_CreateCRPForCertificate_Call) Run(run func(ctx *contexts.Context, cert *certmanagerv1.Certificate, opts createcrpforcertificate.CreateCRPForCertificateOpts)) *MockClientInterface_CreateCRPForCertificate_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*contexts.Context), args[1].(*certmanagerv1.Certificate), args[2].(createcrpforcertificate.CreateCRPForCertificateOpts))
	})
	return _c
}

func (_c *MockClientInterface_CreateCRPForCertificate_Call) Return(_a0 *v1alpha1.CertificateRequestPolicy, _a1 error) *MockClientInterface_CreateCRPForCertificate_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockClientInterface_CreateCRPForCertificate_Call) RunAndReturn(run func(*contexts.Context, *certmanagerv1.Certificate, createcrpforcertificate.CreateCRPForCertificateOpts) (*v1alpha1.CertificateRequestPolicy, error)) *MockClientInterface_CreateCRPForCertificate_Call {
	_c.Call.Return(run)
	return _c
}

// ES provides a mock function with no fields
func (_m *MockClientInterface) ES() externalsnapshotter.ClientInterface {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for ES")
	}

	var r0 externalsnapshotter.ClientInterface
	if rf, ok := ret.Get(0).(func() externalsnapshotter.ClientInterface); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(externalsnapshotter.ClientInterface)
		}
	}

	return r0
}

// MockClientInterface_ES_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ES'
type MockClientInterface_ES_Call struct {
	*mock.Call
}

// ES is a helper method to define mock.On call
func (_e *MockClientInterface_Expecter) ES() *MockClientInterface_ES_Call {
	return &MockClientInterface_ES_Call{Call: _e.mock.On("ES")}
}

func (_c *MockClientInterface_ES_Call) Run(run func()) *MockClientInterface_ES_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockClientInterface_ES_Call) Return(_a0 externalsnapshotter.ClientInterface) *MockClientInterface_ES_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockClientInterface_ES_Call) RunAndReturn(run func() externalsnapshotter.ClientInterface) *MockClientInterface_ES_Call {
	_c.Call.Return(run)
	return _c
}

// NewClusterUserCert provides a mock function with given fields: ctx, namespace, username, issuerName, clusterName, opts
func (_m *MockClientInterface) NewClusterUserCert(ctx *contexts.Context, namespace string, username string, issuerName string, clusterName string, opts clusterusercert.NewClusterUserCertOpts) (clusterusercert.ClusterUserCertInterface, error) {
	ret := _m.Called(ctx, namespace, username, issuerName, clusterName, opts)

	if len(ret) == 0 {
		panic("no return value specified for NewClusterUserCert")
	}

	var r0 clusterusercert.ClusterUserCertInterface
	var r1 error
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, string, string, clusterusercert.NewClusterUserCertOpts) (clusterusercert.ClusterUserCertInterface, error)); ok {
		return rf(ctx, namespace, username, issuerName, clusterName, opts)
	}
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, string, string, clusterusercert.NewClusterUserCertOpts) clusterusercert.ClusterUserCertInterface); ok {
		r0 = rf(ctx, namespace, username, issuerName, clusterName, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(clusterusercert.ClusterUserCertInterface)
		}
	}

	if rf, ok := ret.Get(1).(func(*contexts.Context, string, string, string, string, clusterusercert.NewClusterUserCertOpts) error); ok {
		r1 = rf(ctx, namespace, username, issuerName, clusterName, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockClientInterface_NewClusterUserCert_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'NewClusterUserCert'
type MockClientInterface_NewClusterUserCert_Call struct {
	*mock.Call
}

// NewClusterUserCert is a helper method to define mock.On call
//   - ctx *contexts.Context
//   - namespace string
//   - username string
//   - issuerName string
//   - clusterName string
//   - opts clusterusercert.NewClusterUserCertOpts
func (_e *MockClientInterface_Expecter) NewClusterUserCert(ctx interface{}, namespace interface{}, username interface{}, issuerName interface{}, clusterName interface{}, opts interface{}) *MockClientInterface_NewClusterUserCert_Call {
	return &MockClientInterface_NewClusterUserCert_Call{Call: _e.mock.On("NewClusterUserCert", ctx, namespace, username, issuerName, clusterName, opts)}
}

func (_c *MockClientInterface_NewClusterUserCert_Call) Run(run func(ctx *contexts.Context, namespace string, username string, issuerName string, clusterName string, opts clusterusercert.NewClusterUserCertOpts)) *MockClientInterface_NewClusterUserCert_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*contexts.Context), args[1].(string), args[2].(string), args[3].(string), args[4].(string), args[5].(clusterusercert.NewClusterUserCertOpts))
	})
	return _c
}

func (_c *MockClientInterface_NewClusterUserCert_Call) Return(_a0 clusterusercert.ClusterUserCertInterface, _a1 error) *MockClientInterface_NewClusterUserCert_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockClientInterface_NewClusterUserCert_Call) RunAndReturn(run func(*contexts.Context, string, string, string, string, clusterusercert.NewClusterUserCertOpts) (clusterusercert.ClusterUserCertInterface, error)) *MockClientInterface_NewClusterUserCert_Call {
	_c.Call.Return(run)
	return _c
}

// NewDRVolume provides a mock function with given fields: ctx, namespace, name, configuredSize, opts
func (_m *MockClientInterface) NewDRVolume(ctx *contexts.Context, namespace string, name string, configuredSize resource.Quantity, opts drvolume.DRVolumeCreateOptions) (drvolume.DRVolumeInterface, error) {
	ret := _m.Called(ctx, namespace, name, configuredSize, opts)

	if len(ret) == 0 {
		panic("no return value specified for NewDRVolume")
	}

	var r0 drvolume.DRVolumeInterface
	var r1 error
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, resource.Quantity, drvolume.DRVolumeCreateOptions) (drvolume.DRVolumeInterface, error)); ok {
		return rf(ctx, namespace, name, configuredSize, opts)
	}
	if rf, ok := ret.Get(0).(func(*contexts.Context, string, string, resource.Quantity, drvolume.DRVolumeCreateOptions) drvolume.DRVolumeInterface); ok {
		r0 = rf(ctx, namespace, name, configuredSize, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(drvolume.DRVolumeInterface)
		}
	}

	if rf, ok := ret.Get(1).(func(*contexts.Context, string, string, resource.Quantity, drvolume.DRVolumeCreateOptions) error); ok {
		r1 = rf(ctx, namespace, name, configuredSize, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockClientInterface_NewDRVolume_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'NewDRVolume'
type MockClientInterface_NewDRVolume_Call struct {
	*mock.Call
}

// NewDRVolume is a helper method to define mock.On call
//   - ctx *contexts.Context
//   - namespace string
//   - name string
//   - configuredSize resource.Quantity
//   - opts drvolume.DRVolumeCreateOptions
func (_e *MockClientInterface_Expecter) NewDRVolume(ctx interface{}, namespace interface{}, name interface{}, configuredSize interface{}, opts interface{}) *MockClientInterface_NewDRVolume_Call {
	return &MockClientInterface_NewDRVolume_Call{Call: _e.mock.On("NewDRVolume", ctx, namespace, name, configuredSize, opts)}
}

func (_c *MockClientInterface_NewDRVolume_Call) Run(run func(ctx *contexts.Context, namespace string, name string, configuredSize resource.Quantity, opts drvolume.DRVolumeCreateOptions)) *MockClientInterface_NewDRVolume_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*contexts.Context), args[1].(string), args[2].(string), args[3].(resource.Quantity), args[4].(drvolume.DRVolumeCreateOptions))
	})
	return _c
}

func (_c *MockClientInterface_NewDRVolume_Call) Return(_a0 drvolume.DRVolumeInterface, _a1 error) *MockClientInterface_NewDRVolume_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockClientInterface_NewDRVolume_Call) RunAndReturn(run func(*contexts.Context, string, string, resource.Quantity, drvolume.DRVolumeCreateOptions) (drvolume.DRVolumeInterface, error)) *MockClientInterface_NewDRVolume_Call {
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
