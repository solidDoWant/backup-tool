package helpers

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/gravitational/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	toolswatch "k8s.io/client-go/tools/watch"
	"k8s.io/utils/ptr"
)

func init() {
	yaml.RegisterCustomUnmarshaler(func(mwt *MaxWaitTime, b []byte) error {
		var duration time.Duration
		if err := yaml.Unmarshal(b, &duration); err != nil {
			return err
		}
		*mwt = MaxWaitTime(duration)
		return nil
	})

	yaml.RegisterCustomMarshaler(func(mwt MaxWaitTime) ([]byte, error) {
		return yaml.Marshal(time.Duration(mwt))
	})
}

type metaResource interface {
	GetName() string
	GetNamespace() string
}

func FullName(mr metaResource) string {
	return FullNameStr(mr.GetNamespace(), mr.GetName())
}

func FullNameStr(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}

type GenerateName bool

func (gn GenerateName) SetName(metadata *metav1.ObjectMeta, name string) {
	if gn {
		metadata.GenerateName = CleanName(name)
	} else {
		metadata.Name = name
	}
}

type MaxWaitTime time.Duration

// Very short wait time, mostly used for testing
var ShortWaitTime MaxWaitTime = MaxWaitTime(250 * time.Millisecond)

func (mwt MaxWaitTime) MaxWait(defaultVal time.Duration) time.Duration {
	if mwt == 0 {
		return defaultVal
	}
	return time.Duration(mwt)
}

// Describes a type that can list and watch k8s resources. `TList` should be a list type (such corev1.PodList),
// rather than the listed type (such as corev1.Pod). Typically this should be provided via
// something like `client.CoreV1().Pods(<namespace>)`.
type ListerWatcher[TList runtime.Object] interface {
	List(ctx context.Context, opts metav1.ListOptions) (TList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
}

// Callback for determining if a provided k8s object (`T`, such as corev1.Pod) matches an awaited condition.
// The function returns result `V` (can be `nil`/`interface{}` type if not needed), whether or not the object
// matches the condition, and an error if one occurred during processing.
type WaitEventProcessor[T runtime.Object, V interface{}] func(context.Context, T) (V, bool, error)

// Wait for a check to pass on a given resource, optionally returning a value when the condition passes.
// Will not return until the condition is met, or an error occurs.
func WaitForResourceCondition[T runtime.Object, TList runtime.Object, V interface{}](ctx context.Context, timeout time.Duration, client ListerWatcher[TList], name string, processEvent WaitEventProcessor[T, V]) (V, error) {
	// Setup a timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Setup the k8s API calls
	setCommonOpts := func(options *metav1.ListOptions) {
		options.FieldSelector = fields.OneTermEqualSelector(metav1.ObjectNameField, name).String()
		options.TimeoutSeconds = ptr.To(int64(math.Floor(timeout.Seconds())))
	}

	listFunc := func(options metav1.ListOptions) (runtime.Object, error) {
		setCommonOpts(&options)
		return client.List(timeoutCtx, options)
	}
	watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
		options.Watch = true
		setCommonOpts(&options)
		return client.Watch(timeoutCtx, options)
	}

	var objType T // Due to golang limitations and legacy cruft, this is needed pass around type to some functions
	var result V  // Holds the result from processEvent calls

	// This checks the result of the initial `List` API call to see if a watcher actually needs to be setup.
	initialCheck := func(store cache.Store) (bool, error) {
		items := store.List()
		if len(items) > 1 {
			return false, trace.Errorf("expected at most one item, matched %d", len(items))
		}

		if len(items) == 0 {
			return false, nil
		}

		item := items[0]
		castedItem, ok := item.(T)
		if !ok {
			return false, trace.Errorf("failed to cast item to %T", objType)
		}

		var matched bool
		var err error
		result, matched, err = processEvent(timeoutCtx, castedItem)
		return matched, trace.Wrap(err, "failed while processing initial precondition event")
	}

	// Handles casting the event object to `T`, and the boilerplate logic for calling/returning values from `processEvent`.
	typedProcessEvent := func(event watch.Event) (bool, error) {
		castedItem, ok := event.Object.(T)
		if !ok {
			return false, trace.Errorf("failed to cast item to %T", objType)
		}

		var matched bool
		var err error
		result, matched, err = processEvent(timeoutCtx, castedItem)
		return matched, trace.Wrap(err, "failed while processing initial precondition event")
	}

	_, err := toolswatch.UntilWithSync(
		timeoutCtx,
		&cache.ListWatch{ListFunc: listFunc, WatchFunc: watchFunc},
		objType,
		initialCheck,
		typedProcessEvent,
	)

	return result, trace.Wrap(err, "failed while waiting for condition to become true")
}

// Do a best-effort cleanup of the provided value to make it a valid k8s generated resource name.
func CleanName(generateName string) string {
	// TODO regex
	replaceChars := "_:."
	replacerStrings := make([]string, 0, len(replaceChars)*2)
	for _, char := range replaceChars {
		replacerStrings = append(replacerStrings, string(char), "-")
	}

	return strings.NewReplacer(replacerStrings...).Replace(strings.ToLower(generateName))
}
