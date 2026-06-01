package helpers

import (
	context "context"
	"fmt"
	"math"
	"strings"
	"time"

	"maps"

	"github.com/goccy/go-yaml"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	toolswatch "k8s.io/client-go/tools/watch"
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

func TruncateStringEllipsis(s string, length int) string {
	return TruncateString(s, length, "...")
}

func TruncateString(s string, maxLength int, truncatedSuffix string) string {
	runes := []rune(s)
	if len(runes) <= maxLength {
		return s
	}

	suffixLength := len(truncatedSuffix)
	if maxLength < suffixLength {
		runes := []rune(truncatedSuffix)
		return string(runes[0:maxLength])
	}

	return string(runes[0:maxLength-suffixLength]) + truncatedSuffix
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
// The function returns result `V` (can be `nil`/`any` type if not needed), whether or not the object
// matches the condition, and an error if one occurred during processing.
type WaitEventProcessor[T runtime.Object, V any] func(*contexts.Context, T) (V, bool, error)

// Wait for a check to pass on the single named resource, optionally returning a value when the
// condition passes. Will not return until the condition is met, or an error occurs.
func WaitForResourceCondition[T runtime.Object, TList runtime.Object, V any](ctx *contexts.Context, timeout time.Duration, client ListerWatcher[TList], name string, processEvent WaitEventProcessor[T, V]) (result V, err error) {
	ctx.Log.With("name", name, "timeout", timeout).Debug("Waiting for resource condition")
	defer ctx.Log.Debug("Finished waiting for resource condition", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err), "result", result)

	return waitForResourceCondition(ctx, timeout, client, func(options *metav1.ListOptions) {
		options.FieldSelector = fields.OneTermEqualSelector(metav1.ObjectNameField, name).String()
	}, processEvent)
}

// WaitForResourceConditionByLabel is like WaitForResourceCondition but matches resources by label
// selector instead of name, so it can wait on a resource whose name isn't known ahead of time (such
// as a generate-named Job). The condition is checked against every matched resource and the wait
// returns as soon as any of them satisfies it.
func WaitForResourceConditionByLabel[T runtime.Object, TList runtime.Object, V any](ctx *contexts.Context, timeout time.Duration, client ListerWatcher[TList], labelSelector string, processEvent WaitEventProcessor[T, V]) (result V, err error) {
	ctx.Log.With("labelSelector", labelSelector, "timeout", timeout).Debug("Waiting for resource condition by label")
	defer ctx.Log.Debug("Finished waiting for resource condition by label", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err), "result", result)

	return waitForResourceCondition(ctx, timeout, client, func(options *metav1.ListOptions) {
		options.LabelSelector = labelSelector
	}, processEvent)
}

// waitForResourceCondition List+Watches a resource (selected by setSelector) and runs processEvent
// against the current state and every subsequent change until the condition is met or an error
// occurs. The initial List is checked across all matched items, so it works for both single-name and
// label-selected waits.
func waitForResourceCondition[T runtime.Object, TList runtime.Object, V any](ctx *contexts.Context, timeout time.Duration, client ListerWatcher[TList], setSelector func(*metav1.ListOptions), processEvent WaitEventProcessor[T, V]) (result V, err error) {
	// Setup a timeout context for processing events
	eventCtx, cancel := ctx.Child().WithTimeout(timeout)
	defer cancel()

	// Setup the k8s API calls
	setCommonOpts := func(options *metav1.ListOptions) {
		setSelector(options)
		options.TimeoutSeconds = new(int64(math.Floor(timeout.Seconds())))
	}

	listFunc := func(options metav1.ListOptions) (runtime.Object, error) {
		setCommonOpts(&options)
		return client.List(eventCtx, options)
	}
	watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
		options.Watch = true
		setCommonOpts(&options)
		return client.Watch(eventCtx, options)
	}

	var objType T // Due to golang limitations and legacy cruft, this is needed pass around type to some functions

	// This checks the result of the initial `List` API call to see if a watcher actually needs to be setup.
	initialCheck := func(store cache.Store) (matched bool, err error) {
		eventCtx.Log.Debug("Checking initial resource condition")
		defer eventCtx.Log.Debug("Initial condition check results", "matched", matched, contexts.ErrorKeyvals(&err))

		for _, item := range store.List() {
			castedItem, ok := item.(T)
			if !ok {
				return false, trace.Errorf("failed to cast item to %T", objType)
			}

			result, matched, err = processEvent(eventCtx.Child(), castedItem)
			if matched || err != nil {
				return matched, trace.Wrap(err, "failed while processing initial precondition event")
			}
		}
		return false, nil
	}

	// Handles casting the event object to `T`, and the boilerplate logic for calling/returning values from `processEvent`.
	typedProcessEvent := func(event watch.Event) (matched bool, err error) {
		eventCtx.Log.Debug("Processing event")
		defer eventCtx.Log.Debug("Processed event", "matched", matched, contexts.ErrorKeyvals(&err))

		castedItem, ok := event.Object.(T)
		if !ok {
			return false, trace.Errorf("failed to cast item to %T", objType)
		}
		eventCtx.Log.With("item", castedItem)

		result, matched, err = processEvent(eventCtx.Child(), castedItem)
		return matched, trace.Wrap(err, "failed while processing event")
	}

	_, err = toolswatch.UntilWithSync(
		eventCtx,
		&cache.ListWatch{ListFunc: listFunc, WatchFunc: watchFunc},
		objType,
		initialCheck,
		typedProcessEvent,
	)

	return result, trace.Wrap(err, "failed while waiting for condition to become true")
}

// WaitForResourceDeletion waits until the named resource no longer exists, returning immediately if it
// is already gone. Like WaitForResourceCondition it List+Watches (rather than polling) for consistency
// with the rest of the project; WaitForResourceCondition itself can't be reused here because it treats
// an empty initial list as "keep waiting" and never surfaces the delete event to its callback.
func WaitForResourceDeletion[T runtime.Object, TList runtime.Object](ctx *contexts.Context, timeout time.Duration, client ListerWatcher[TList], name string) (err error) {
	ctx.Log.With("name", name, "timeout", timeout).Debug("Waiting for resource deletion")
	defer ctx.Log.Debug("Finished waiting for resource deletion", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	eventCtx, cancel := ctx.Child().WithTimeout(timeout)
	defer cancel()

	setCommonOpts := func(options *metav1.ListOptions) {
		options.FieldSelector = fields.OneTermEqualSelector(metav1.ObjectNameField, name).String()
		options.TimeoutSeconds = new(int64(math.Floor(timeout.Seconds())))
	}
	listFunc := func(options metav1.ListOptions) (runtime.Object, error) {
		setCommonOpts(&options)
		return client.List(eventCtx, options)
	}
	watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
		options.Watch = true
		setCommonOpts(&options)
		return client.Watch(eventCtx, options)
	}

	var objType T

	// Already deleted if the initial list turns up nothing.
	initialCheck := func(store cache.Store) (bool, error) {
		return len(store.List()) == 0, nil
	}
	// Otherwise, done once the delete event arrives.
	processEvent := func(event watch.Event) (bool, error) {
		return event.Type == watch.Deleted, nil
	}

	_, err = toolswatch.UntilWithSync(
		eventCtx,
		&cache.ListWatch{ListFunc: listFunc, WatchFunc: watchFunc},
		objType,
		initialCheck,
		processEvent,
	)

	return trace.Wrap(err, "failed while waiting for resource %q to be deleted", name)
}

// Do a best-effort cleanup of the provided value to make it a valid k8s generated resource name.
func CleanName(generateName string) string {
	replaceChars := "_:."
	replacerStrings := make([]string, 0, len(replaceChars)*2)
	for _, char := range replaceChars {
		replacerStrings = append(replacerStrings, string(char), "-")
	}

	cleanedName := strings.NewReplacer(replacerStrings...).Replace(strings.ToLower(generateName))
	for i := len(cleanedName) - 1; i >= 0; i-- {
		if cleanedName[i] != '-' {
			break
		}

		// Trim the last character if it is a `-`
		cleanedName = cleanedName[:i]
	}

	return cleanedName
}

// Describes a type that can label k8s resources.
// Used to set common labels on resources, which is important for integration
// with external systems like netpols.
type ResourceLabeler interface {
	SetCommonLabels(labels map[string]string)
}

// This is a subset of metav1.Object that only includes label functions.
type LabelableResource interface {
	GetLabels() map[string]string
	SetLabels(labels map[string]string)
}

type SimpleResourceLabeler struct {
	CommonLabels map[string]string
}

func (srl *SimpleResourceLabeler) SetCommonLabels(labels map[string]string) {
	srl.CommonLabels = labels
}

// Label a resource with the common labels provided to the labeler, if labels with
// the same keys do not already exist on the resource.
func (srl SimpleResourceLabeler) LabelResource(resource LabelableResource) {
	if srl.CommonLabels == nil {
		return
	}

	combinedLabels := make(map[string]string, len(resource.GetLabels())+len(srl.CommonLabels))
	maps.Copy(combinedLabels, srl.CommonLabels)
	maps.Copy(combinedLabels, resource.GetLabels())
	resource.SetLabels(combinedLabels)
}
