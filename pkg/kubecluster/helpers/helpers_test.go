package helpers

import (
	"sync"
	"testing"
	"time"

	"github.com/solidDoWant/backup-tool/pkg/contexts"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func testFullNameImplentation(t *testing.T, call func(namespace, name string) string) {
	tests := []struct {
		desc      string
		namespace string
		name      string
		want      string
	}{
		{
			desc:      "basic resource",
			namespace: "default",
			name:      "test-resource",
			want:      "default/test-resource",
		},
		{
			desc:      "empty namespace",
			namespace: "",
			name:      "test-resource",
			want:      "/test-resource",
		},
		{
			desc:      "empty name",
			namespace: "default",
			name:      "",
			want:      "default/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := call(tt.namespace, tt.name)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFullName(t *testing.T) {
	testFullNameImplentation(t, func(namespace, name string) string {
		resource := NewMockmetaResource(t)
		resource.EXPECT().GetNamespace().Return(namespace)
		resource.EXPECT().GetName().Return(name)

		return FullName(resource)
	})
}

func TestFullNameStr(t *testing.T) {
	testFullNameImplentation(t, FullNameStr)
}

func TestGenerateNameSetName(t *testing.T) {
	tests := []struct {
		desc         string
		generateName GenerateName
		name         string
		wantName     string
		wantGenName  string
	}{
		{
			desc:         "with generate name true",
			generateName: true,
			name:         "test",
			wantGenName:  "test",
		},
		{
			desc:     "with generate name false",
			name:     "test",
			wantName: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			metadata := &metav1.ObjectMeta{}

			tt.generateName.SetName(metadata, tt.name)

			assert.Equal(t, tt.wantName, metadata.Name)
			assert.Equal(t, tt.wantGenName, metadata.GenerateName)
		})
	}
}

func TestMaxWaitShortWaitTIme(t *testing.T) {
	require.LessOrEqual(t, ShortWaitTime, time.Second)
}

func TestMaxWaitTimeMaxWait(t *testing.T) {
	defaultVal := time.Second * 5

	tests := []struct {
		desc     string
		mwt      MaxWaitTime
		default_ time.Duration
		want     time.Duration
	}{
		{
			desc:     "should return default value when MaxWaitTime is 0",
			mwt:      MaxWaitTime(0),
			default_: defaultVal,
			want:     defaultVal,
		},
		{
			desc:     "should return MaxWaitTime value when not 0",
			mwt:      MaxWaitTime(time.Second * 10),
			default_: defaultVal,
			want:     time.Second * 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := tt.mwt.MaxWait(tt.default_)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWaitForResourceCondition(t *testing.T) {
	// Helpers for test cases
	podIP := "1.2.3.4"

	namespace := "test-ns"
	resourceName := "test-resource"

	matchingPodWithIP := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      resourceName,
		},
		Status: corev1.PodStatus{
			PodIP: podIP,
		},
	}

	matchingPodWithoutIP := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      resourceName,
		},
	}

	matchingNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: resourceName,
		},
	}

	// Test cases
	tests := []struct {
		desc                string
		initialResources    []runtime.Object
		useWrongListerType  bool
		useWrongWatcherType bool
		processEvent        WaitEventProcessor[*corev1.Pod, string]
		afterStartedWaiting func(*testing.T, *contexts.Context, k8s.Interface)
		expectedResult      string
		wantErr             bool
	}{
		{
			desc:    "resource never exists",
			wantErr: true,
		},
		{
			desc:             "pod matches but condition never met",
			initialResources: []runtime.Object{matchingPodWithoutIP},
			wantErr:          true,
		},
		{
			desc:             "pod matches and condition initially met",
			initialResources: []runtime.Object{matchingPodWithIP},
			expectedResult:   podIP,
		},
		{
			desc:           "pod doesnt initially exist but is added later",
			expectedResult: podIP,
			afterStartedWaiting: func(t *testing.T, ctx *contexts.Context, client k8s.Interface) {
				_, err := client.CoreV1().Pods(namespace).Create(ctx, matchingPodWithIP, metav1.CreateOptions{})
				require.NoError(t, err)
			},
		},
		{
			desc:             "pod doesnt initially exist without matching, but matches later",
			expectedResult:   podIP,
			initialResources: []runtime.Object{matchingPodWithoutIP},
			afterStartedWaiting: func(t *testing.T, ctx *contexts.Context, client k8s.Interface) {
				_, err := client.CoreV1().Pods(namespace).Update(ctx, matchingPodWithIP, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
		{
			desc:               "pod matches but lister is the wrong type",
			initialResources:   []runtime.Object{matchingNamespace},
			useWrongListerType: true,
			wantErr:            true,
		},
		{
			desc: "pod matches but lister is the wrong type",
			afterStartedWaiting: func(t *testing.T, ctx *contexts.Context, client k8s.Interface) {
				_, err := client.CoreV1().Namespaces().Create(ctx, matchingNamespace, metav1.CreateOptions{})
				require.NoError(t, err)
			},
			useWrongWatcherType: true,
			wantErr:             true,
		},
		{
			desc:             "pod initially exists without matching, and then is deleted",
			initialResources: []runtime.Object{matchingPodWithoutIP},
			afterStartedWaiting: func(t *testing.T, ctx *contexts.Context, client k8s.Interface) {
				err := client.CoreV1().Pods(namespace).Delete(ctx, matchingPodWithIP.Name, metav1.DeleteOptions{})
				require.NoError(t, err)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			client := fake.NewClientset(tt.initialResources...)
			ctx := th.NewTestContext()

			if tt.processEvent == nil {
				// Default implementation that should be logically sound
				tt.processEvent = func(ctx *contexts.Context, pod *corev1.Pod) (string, bool, error) {
					if pod.Status.PodIP == "" {
						return "", false, nil
					}

					return pod.Status.PodIP, true, nil
				}
			}

			// Prepare to run the function in another goroutine so that events can be passed concurrently
			var wg sync.WaitGroup
			var result string
			var waitErr error
			wg.Add(1)

			// Build the ListerGetter instance to use
			// This must be tested because there is nothing to tie the resource type to the resource list type
			// (such as pod to podlist) at compile time
			// This is a mess due to Go generics limitations. Each branch technically has a different function signature
			if tt.useWrongListerType {
				lw := NewMockListerWatcher[*corev1.NamespaceList](t)
				lw.EXPECT().List(mock.Anything, mock.Anything).RunAndReturn(client.CoreV1().Namespaces().List)
				lw.EXPECT().Watch(mock.Anything, mock.Anything).RunAndReturn(client.CoreV1().Pods(namespace).Watch)

				go func() {
					result, waitErr = WaitForResourceCondition(ctx, time.Second, lw, resourceName, tt.processEvent)
					wg.Done()
				}()
			} else {
				lw := NewMockListerWatcher[*corev1.PodList](t)
				lw.EXPECT().List(mock.Anything, mock.Anything).RunAndReturn(client.CoreV1().Pods(namespace).List)

				if tt.useWrongWatcherType {
					lw.EXPECT().Watch(mock.Anything, mock.Anything).RunAndReturn(client.CoreV1().Namespaces().Watch)
				} else {
					lw.EXPECT().Watch(mock.Anything, mock.Anything).RunAndReturn(client.CoreV1().Pods(namespace).Watch)
				}

				go func() {
					result, waitErr = WaitForResourceCondition(ctx, time.Second, lw, resourceName, tt.processEvent)
					wg.Done()
				}()
			}

			if tt.afterStartedWaiting != nil {
				time.Sleep(10 * time.Millisecond) // Ensure that watcher has been setup
				tt.afterStartedWaiting(t, ctx, client)
			}

			wg.Wait()

			if tt.wantErr {
				assert.Error(t, waitErr)
				return
			}

			assert.NoError(t, waitErr)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestCleanName(t *testing.T) {
	tests := []struct {
		desc  string
		input string
		want  string
	}{
		{
			desc:  "should handle valid name",
			input: "test-name",
			want:  "test-name",
		},
		{
			desc:  "should replace underscores with hyphens",
			input: "test_name",
			want:  "test-name",
		},
		{
			desc:  "should replace colons with hyphens",
			input: "test:name",
			want:  "test-name",
		},
		{
			desc:  "should replace dots with hyphens",
			input: "test.name",
			want:  "test-name",
		},
		{
			desc:  "should convert to lowercase",
			input: "TestName",
			want:  "testname",
		},
		{
			desc:  "should handle multiple replacements",
			input: "Test_Name:Suffix",
			want:  "test-name-suffix",
		},
		{
			desc:  "should handle empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			assert.Equal(t, tt.want, CleanName(tt.input))
		})
	}
}
