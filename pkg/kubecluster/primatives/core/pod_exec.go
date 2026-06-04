package core

import (
	"bytes"
	"io"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// ExecInPod runs command in a container of an existing pod and returns its stdout and stderr. It is
// the in-cluster equivalent of `kubectl exec`: it streams over SPDY using the rest config, so it does
// not require credentials, a Service, or pod-network reachability to the workload. It is used to run
// psql against a CNPG primary over the pod's local socket (authenticating as the in-pod superuser)
// without issuing client certificates.
func (c *Client) ExecInPod(ctx *contexts.Context, namespace, podName, container string, command []string, stdin io.Reader) (string, string, error) {
	ctx.Log.With("pod", helpers.FullNameStr(namespace, podName), "container", container).Debug("Executing command in pod", "command", command)

	request := c.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   command,
			Stdin:     stdin != nil,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(c.restConfig, "POST", request.URL())
	if err != nil {
		return "", "", trace.Wrap(err, "failed to create executor for pod %q", helpers.FullNameStr(namespace, podName))
	}

	var stdout, stderr bytes.Buffer
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: &stdout,
		Stderr: &stderr,
	})

	return stdout.String(), stderr.String(), trace.Wrap(err, "failed to exec %v in pod %q: %s", command, helpers.FullNameStr(namespace, podName), stderr.String())
}
