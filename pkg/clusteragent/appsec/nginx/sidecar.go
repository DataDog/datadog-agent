// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package nginx

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

const (
	initContainerName  = "datadog-appsec-nginx-init"
	moduleVolumeName   = "datadog-appsec-nginx-modules"
	configmapArgPrefix = "--configmap="

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

// ShouldMutatePod returns true if the pod is an ingress-nginx controller pod
// that hasn't already been injected
func (n *nginxSidecarPattern) ShouldMutatePod(pod *corev1.Pod) bool {
	if pod.Labels[labelNameKey] != labelNameValue {
		return false
	}
	if pod.Labels[labelComponentKey] != labelComponentValue {
		return false
	}
	if hasInitContainer(pod) {
		n.logger.Debugf("Pod %s already has nginx appsec init container", mutatecommon.PodString(pod))
		return false
	}
	return true
}

// IsNamespaceEligible returns true for all namespaces since ingress-nginx
// controller pods can run in any namespace
func (n *nginxSidecarPattern) IsNamespaceEligible(string) bool {
	return true
}

// MutatePod injects the nginx-datadog module into an ingress-nginx controller pod by:
// 1. Adding an init container that copies the .so module
// 2. Adding an emptyDir volume for module sharing
// 3. Redirecting the --configmap arg to a DD-owned ConfigMap
func (n *nginxSidecarPattern) MutatePod(pod *corev1.Pod, ns string, client dynamic.Interface) (bool, error) {
	// Find the controller container with --configmap arg (or note it's absent)
	containerIdx, argIdx, cmNamespace, cmName, found := findControllerConfigMapArg(pod, ns)
	if !found {
		cmName = "ingress-nginx-controller"
		cmNamespace = ns
	}

	// Parse version from controller image
	container := &pod.Spec.Containers[containerIdx]
	version, err := parseControllerVersion(container.Image)
	if err != nil {
		n.eventRecorder.recordVersionParseFailed(pod.Name, container.Image)
		return false, fmt.Errorf("failed to parse ingress-nginx version from image %q: %w. Follow the manual extraModules process to enable AppSec", container.Image, err)
	}

	moduleMountPath := n.config.Nginx.ModuleMountPath

	// Use common labels and add proxy-type for label-based cleanup filtering
	ddLabels := make(map[string]string, len(n.config.CommonLabels)+1)
	maps.Copy(ddLabels, n.config.CommonLabels)
	ddLabels[appsecconfig.AppsecProcessorProxyTypeAnnotation] = string(appsecconfig.ProxyTypeIngressNginx)
	ddCMName := ddConfigMapName(cmName)
	// context.Background: MutatePod interface does not provide a context.
	// The admission webhook timeout still bounds the overall request.
	if err := createOrUpdateDDConfigMap(context.Background(), client, cmNamespace, cmName, moduleMountPath, ddLabels, n.config.CommonAnnotations); err != nil {
		// IngressClass name is not available during pod mutation; empty name is
		// acceptable because the event message contains the ConfigMap name.
		n.eventRecorder.recordConfigMapCreateFailed("", err)
		return false, fmt.Errorf("failed to create/update DD ConfigMap: %w", err)
	}
	// IngressClass name is not available during pod mutation; empty name is
	// acceptable because the event message contains the ConfigMap name.
	n.eventRecorder.recordConfigMapCreated("", ddCMName)
	n.logger.Infof("Created/updated DD ConfigMap %s/%s", cmNamespace, ddCMName)

	// Add emptyDir volume for module sharing
	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
		Name: moduleVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	// Add init container that copies the .so module
	pod.Spec.InitContainers = append(pod.Spec.InitContainers, buildInitContainer(n.config.Nginx.InitImage, version, moduleMountPath))

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

	n.logger.Infof("Injected nginx-datadog module into pod %s (version %s)", mutatecommon.PodString(pod), version)

	return true, nil
}

// PodDeleted is a no-op for nginx. Cleanup is handled by Deleted() on IngressClass.
func (n *nginxSidecarPattern) PodDeleted(*corev1.Pod, string, dynamic.Interface) (bool, error) {
	return false, nil
}

// MatchCondition returns a CEL expression for server-side pod filtering.
// It checks for ingress-nginx controller labels safely (existence before access).
func (n *nginxSidecarPattern) MatchCondition() admissionregistrationv1.MatchCondition {
	return admissionregistrationv1.MatchCondition{
		Expression: "'" + labelNameKey + "' in object.metadata.labels && " +
			"object.metadata.labels['" + labelNameKey + "'] == '" + labelNameValue + "' && " +
			"'" + labelComponentKey + "' in object.metadata.labels && " +
			"object.metadata.labels['" + labelComponentKey + "'] == '" + labelComponentValue + "'",
	}
}

// Added delegates to the base pattern (no-op for nginx sidecar mode)
func (n *nginxSidecarPattern) Added(ctx context.Context, obj *unstructured.Unstructured) error {
	return n.nginxInjectionPattern.Added(ctx, obj)
}

// findControllerConfigMapArg finds the controller container and its --configmap arg,
// resolving $(POD_NAMESPACE) to the actual pod namespace.
// If the arg is not found, found is false and containerIdx 0 / argIdx -1 are returned
// so the caller can append the arg to the first container instead.
func findControllerConfigMapArg(pod *corev1.Pod, podNamespace string) (containerIdx, argIdx int, cmNamespace, cmName string, found bool) {
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
			// Resolve $(POD_NAMESPACE) to the actual namespace
			if ns == "$(POD_NAMESPACE)" {
				ns = podNamespace
			}
			return ci, ai, ns, name, true
		}
	}
	return 0, -1, podNamespace, "", false
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

// buildInitContainer creates the init container spec that copies the nginx-datadog module
func buildInitContainer(initImage, version, moduleMountPath string) corev1.Container {
	return corev1.Container{
		Name:    initContainerName,
		Image:   initImage + ":" + version,
		Command: []string{"/bin/sh", "/datadog/init_module.sh", moduleMountPath},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      moduleVolumeName,
				MountPath: moduleMountPath,
			},
		},
	}
}
