// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package nginx

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/distribution/reference"

	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/dynamic"
	"k8s.io/utils/ptr"
)

// errCrossNamespaceConfigMap signals that the pod's --configmap arg references
// a namespace different from the pod's own. We must not act on this because
// the DCA service account holds cluster-wide ConfigMap permissions and the pod
// creator may be a low-privileged tenant.
var errCrossNamespaceConfigMap = &appsecconfig.MutationSkippedReason{Reason: appsecconfig.SkipReasonCrossNamespaceConfigMap}

// errEmptyConfigMapName signals that the pod's --configmap arg has an empty
// name after the slash (e.g. "--configmap=foo/"). This is a malformed arg
// and we refuse to act on it.
var errEmptyConfigMapName = &appsecconfig.MutationSkippedReason{Reason: appsecconfig.SkipReasonInvalidConfigMapArg}

const (
	// mutateTimeout bounds ConfigMap operations during pod mutation to prevent
	// goroutine leaks if the API server is slow. The MutatePod interface does not
	// provide a context, so we create one with an explicit timeout.
	mutateTimeout = 10 * time.Second

	initContainerName        = "datadog-appsec-nginx-init"
	moduleVolumeName         = "datadog-appsec-nginx-modules"
	configmapArgPrefix       = "--configmap="
	controllerClassArgPrefix = "--controller-class="

	labelNameKey        = "app.kubernetes.io/name"
	labelNameValue      = "ingress-nginx"
	labelComponentKey   = "app.kubernetes.io/component"
	labelComponentValue = "controller"
)

var _ appsecconfig.SidecarInjectionPattern = (*nginxSidecarPattern)(nil)

// nginxSidecarPattern implements SidecarInjectionPattern for ingress-nginx pod mutation.
// Despite its name (inherited from the interface), this pattern injects an init container
// and redirects the --configmap arg, rather than adding a sidecar container.
type nginxSidecarPattern struct {
	*nginxInjectionPattern
}

func (n *nginxSidecarPattern) IsPodEligible(pod *corev1.Pod, _ string) bool {
	labelsMatch := pod.Labels[labelNameKey] == labelNameValue && pod.Labels[labelComponentKey] == labelComponentValue
	if !labelsMatch {
		if !hasIngressNginxControllerClassArg(pod) {
			return false
		}
		n.logger.Warnf("Pod %s matched by --controller-class arg but missing standard labels; consider adding %s=%s and %s=%s labels",
			mutatecommon.PodString(pod), labelNameKey, labelNameValue, labelComponentKey, labelComponentValue)
	}
	return true
}

// MutatePod injects the nginx-datadog module into an ingress-nginx controller pod by:
// 1. Adding an init container that copies the .so module
// 2. Adding an emptyDir volume for module sharing
// 3. Redirecting the --configmap arg to a DD-owned ConfigMap
func (n *nginxSidecarPattern) MutatePod(pod *corev1.Pod, ns string, client dynamic.Interface) (appsecconfig.MutationOutcome, error) {
	if hasInitContainer(pod) {
		return appsecconfig.MutationSkipped, &appsecconfig.MutationSkippedReason{Reason: appsecconfig.SkipReasonAlreadyInitSidecar}
	}
	if len(pod.Spec.Containers) == 0 {
		return appsecconfig.MutationError, fmt.Errorf("pod %s has no containers", mutatecommon.PodString(pod))
	}

	containerIdx, argIdx, cmNamespace, cmName, found, err := findControllerConfigMapArg(pod, ns)
	if err != nil {
		var skipped *appsecconfig.MutationSkippedReason
		if errors.As(err, &skipped) {
			n.eventRecorder.recordCrossNamespaceConfigMapRefused(pod, err)
			return appsecconfig.MutationSkipped, skipped
		}
		return appsecconfig.MutationError, fmt.Errorf("nginx appsec: failed to resolve controller configmap arg for pod %s: %w", mutatecommon.PodString(pod), err)
	}
	if !found {
		cmName = "ingress-nginx-controller"
		cmNamespace = ns
	}

	container := &pod.Spec.Containers[containerIdx]

	initImageRef := n.config.Nginx.InitImage
	if !imageIsFullyQualified(initImageRef) {
		version, err := parseControllerVersion(container.Image)
		if err != nil {
			n.eventRecorder.recordVersionParseFailed(pod.Name, container.Image)
			return appsecconfig.MutationError, fmt.Errorf("failed to parse ingress-nginx version from image %q: %w. Follow the manual extraModules process to enable AppSec", container.Image, err)
		}
		initImageRef = initImageRef + ":" + version
	}

	moduleMountPath := n.config.Nginx.ModuleMountPath

	// Use common labels and add proxy-type for label-based cleanup filtering
	ddLabels := make(map[string]string, len(n.config.CommonLabels)+1)
	maps.Copy(ddLabels, n.config.CommonLabels)
	ddLabels[appsecconfig.AppsecProcessorProxyTypeAnnotation] = string(appsecconfig.ProxyTypeIngressNginx)
	ddCMName := ddConfigMapName(cmName)
	// MutatePod interface does not provide a context. Use a bounded timeout
	// to prevent goroutine leaks if the API server is slow.
	mutateCtx, cancel := context.WithTimeout(context.Background(), mutateTimeout)
	defer cancel()
	if err := createOrUpdateDDConfigMap(mutateCtx, client, cmNamespace, cmName, moduleMountPath, ddLabels, n.config.CommonAnnotations); err != nil {
		// IngressClass name is not available during pod mutation; empty name is
		// acceptable because the event message contains the ConfigMap name.
		n.eventRecorder.recordConfigMapCreateFailed("", err)
		return appsecconfig.MutationError, fmt.Errorf("failed to create/update DD ConfigMap: %w", err)
	}
	// IngressClass name is not available during pod mutation; empty name is
	// acceptable because the event message contains the ConfigMap name.
	n.eventRecorder.recordConfigMapCreated("", ddCMName)
	n.logger.Infof("Created/updated DD ConfigMap %s/%s", cmNamespace, ddCMName)

	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
		Name: moduleVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				SizeLimit: resource.NewScaledQuantity(50, resource.Mega),
			},
		},
	})

	pod.Spec.InitContainers = append(pod.Spec.InitContainers, buildInitContainer(initImageRef, moduleMountPath, n.config.Nginx.InitRunAsUser, n.config.Nginx.InitRunAsGroup))

	// Add volume mount to controller container
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      moduleVolumeName,
		MountPath: moduleMountPath,
	})

	// Redirect --configmap arg to DD-owned ConfigMap (or add it if absent)
	if found {
		container.Args[argIdx] = fmt.Sprintf("%s%s/%s", configmapArgPrefix, cmNamespace, ddCMName)
	} else {
		container.Args = append(container.Args, fmt.Sprintf("%s%s/%s", configmapArgPrefix, cmNamespace, ddCMName))
	}

	n.logger.Infof("Injected nginx-datadog module into pod %s (image %s)", mutatecommon.PodString(pod), initImageRef)

	return appsecconfig.MutationMutated, nil
}

func (n *nginxSidecarPattern) PodDeleted(*corev1.Pod, string, dynamic.Interface) (appsecconfig.MutationOutcome, error) {
	// PodDeleted is a no-op; the returned outcome is only consulted for the DELETE admission error path (the metric is not emitted on delete).
	return appsecconfig.MutationMutated, nil
}

// MatchCondition returns a CEL expression for server-side pod filtering.
// It checks for ingress-nginx controller labels safely (existence before access).
func (n *nginxSidecarPattern) MatchCondition() admissionregistrationv1.MatchCondition {
	const labelContainsFmt = "'%s' in object.metadata.labels && object.metadata.labels['%s'] == '%s'"
	labelCheck := fmt.Sprintf(labelContainsFmt, labelNameKey, labelNameKey, labelNameValue) +
		" && " + fmt.Sprintf(labelContainsFmt, labelComponentKey, labelComponentKey, labelComponentValue)
	argsCheck := fmt.Sprintf(
		"object.spec.containers.exists(c, c.args.exists(a, a.startsWith('%s%s')))",
		controllerClassArgPrefix, ingressNginxControllerName,
	)
	return admissionregistrationv1.MatchCondition{
		Expression: fmt.Sprintf("(%s) || (%s)", labelCheck, argsCheck),
	}
}

// findControllerConfigMapArg finds the controller container and its --configmap arg.
// It resolves $(POD_NAMESPACE) to the pod's namespace and rejects any other
// namespace value, because the pod arg is attacker-controlled and the DCA holds
// cluster-wide ConfigMap permissions (confused-deputy primitive). It also
// rejects empty names (after the slash separator).
//
// Return contract:
//   - arg absent: found=false, err=nil — caller defaults to (podNamespace, "ingress-nginx-controller").
//   - arg present and valid: found=true, err=nil.
//   - arg present but malformed/cross-namespace: err!=nil — caller must skip mutation.
//
// The webhook runs before kubelet substitution, so "$(POD_NAMESPACE)" arrives
// as a literal string and we resolve it ourselves. Upstream ingress-nginx only
// supports this single syntax, so variants like ${POD_NAMESPACE} are not recognized.
func findControllerConfigMapArg(pod *corev1.Pod, podNamespace string) (containerIdx, argIdx int, cmNamespace, cmName string, found bool, err error) {
	for ci, c := range pod.Spec.Containers {
		for ai, arg := range c.Args {
			value, ok := strings.CutPrefix(arg, configmapArgPrefix)
			if !ok {
				continue
			}
			ns, name, ok := strings.Cut(value, "/")
			if !ok {
				continue
			}
			if ns == "$(POD_NAMESPACE)" {
				ns = podNamespace
			}
			if ns != podNamespace {
				return ci, ai, "", "", false, fmt.Errorf("%w: pod %s, arg %q",
					errCrossNamespaceConfigMap, mutatecommon.PodString(pod), arg)
			}
			if name == "" {
				return ci, ai, "", "", false, fmt.Errorf("%w: pod %s, arg %q",
					errEmptyConfigMapName, mutatecommon.PodString(pod), arg)
			}
			return ci, ai, ns, name, true, nil
		}
	}
	return 0, -1, podNamespace, "", false, nil
}

// parseControllerVersion extracts the version tag from an ingress-nginx controller image reference.
// Examples:
//
//	"registry.k8s.io/ingress-nginx/controller:v1.15.1@sha256:abc" -> "v1.15.1"
//	"registry.k8s.io/ingress-nginx/controller:v1.10.0" -> "v1.10.0"
func parseControllerVersion(image string) (string, error) {
	// Strip digest if present
	image, _, _ = strings.Cut(image, "@")

	// Find tag after last colon
	idx := strings.LastIndex(image, ":")
	if idx == -1 {
		return "", fmt.Errorf("no tag found in image %q", image)
	}
	tag := image[idx+1:]
	if tag == "" || tag == "latest" {
		return "", fmt.Errorf("cannot determine nginx version from tag %q", tag)
	}
	// Guard against registry:port being parsed as a tag (e.g. "myregistry.com:5000/org/image")
	if strings.Contains(tag, "/") {
		return "", fmt.Errorf("no tag found in image %q", image)
	}

	return tag, nil
}

// hasInitContainer checks if the DD init container is already present
func hasInitContainer(pod *corev1.Pod) bool {
	return slices.ContainsFunc(pod.Spec.InitContainers, func(c corev1.Container) bool {
		return c.Name == initContainerName
	})
}

// hasIngressNginxControllerClassArg checks if any container has a --controller-class arg
// matching the ingress-nginx controller name, used as a fallback when standard labels are absent.
func hasIngressNginxControllerClassArg(pod *corev1.Pod) bool {
	for _, c := range pod.Spec.Containers {
		for _, arg := range c.Args {
			if strings.HasPrefix(arg, controllerClassArgPrefix+ingressNginxControllerName) {
				return true
			}
		}
	}
	return false
}

// imageIsFullyQualified reports whether image already carries a tag or digest,
// meaning it should be used as-is without appending a runtime-derived tag.
func imageIsFullyQualified(image string) bool {
	ref, err := reference.Parse(image)
	if err != nil {
		return false
	}
	_, hasTag := ref.(reference.Tagged)
	_, hasDigest := ref.(reference.Digested)
	return hasTag || hasDigest
}

// buildInitContainer creates the init container spec that copies the nginx-datadog module.
// image must be a fully-qualified image reference including the tag (e.g. "repo/image:tag").
//
// runAsUser/runAsGroup set the container's non-root identity. The stock init
// image declares no USER (runs as root), which the kubelet rejects under
// RunAsNonRoot ("container has runAsNonRoot and image will run as root"), so a
// non-root UID/GID is required for it. A negative value leaves the respective
// field unset so a custom image's own USER is honored.
func buildInitContainer(image, moduleMountPath string, runAsUser, runAsGroup int64) corev1.Container {
	sc := &corev1.SecurityContext{
		RunAsNonRoot:             ptr.To(true),
		AllowPrivilegeEscalation: ptr.To(false),
		ReadOnlyRootFilesystem:   ptr.To(true),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}
	if runAsUser >= 0 {
		sc.RunAsUser = ptr.To(runAsUser)
	}
	if runAsGroup >= 0 {
		sc.RunAsGroup = ptr.To(runAsGroup)
	}
	return corev1.Container{
		Name:    initContainerName,
		Image:   image,
		Command: []string{"/bin/sh", "/datadog/init_module.sh", moduleMountPath},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      moduleVolumeName,
				MountPath: moduleMountPath,
			},
		},
		SecurityContext: sc,
	}
}
