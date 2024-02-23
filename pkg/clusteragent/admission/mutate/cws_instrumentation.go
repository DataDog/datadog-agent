// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package mutate

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usersessions"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	cwsVolumeName                        = "datadog-cws-instrumentation"
	cwsMountPath                         = "/datadog-cws-instrumentation"
	cwsInstrumentationPodAnotationStatus = "admission.datadoghq.com/cws-instrumentation.status"
	cwsInstrumentationPodAnotationReady  = "ready"
	cwsInjectorInitContainerName         = "cws-instrumentation"
	cwsUserSessionDataMaxSize            = 1024

	// CWSInstrumentationPodLabelEnabled is used to label pods that should be instrumented or skipped by the CWS mutating webhook
	CWSInstrumentationPodLabelEnabled = "admission.datadoghq.com/cws-instrumentation.enabled"
)

func parseCWSInitContainerResources() (*corev1.ResourceRequirements, error) {
	var resources = &corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}}
	if cpu := config.Datadog.GetString("admission_controller.cws_instrumentation.init_resources.cpu"); len(cpu) > 0 {
		quantity, err := resource.ParseQuantity(cpu)
		if err != nil {
			return nil, err
		}
		resources.Requests[corev1.ResourceCPU] = quantity
		resources.Limits[corev1.ResourceCPU] = quantity
	}

	if mem := config.Datadog.GetString("admission_controller.cws_instrumentation.init_resources.memory"); len(mem) > 0 {
		quantity, err := resource.ParseQuantity(mem)
		if err != nil {
			return nil, err
		}
		resources.Requests[corev1.ResourceMemory] = quantity
		resources.Limits[corev1.ResourceMemory] = quantity
	}

	if len(resources.Limits) > 0 || len(resources.Requests) > 0 {
		return resources, nil
	}
	return nil, nil
}

// isPodCWSInstrumentationReady returns true if the pod has already been instrumented by CWS
func isPodCWSInstrumentationReady(annotations map[string]string) bool {
	return annotations[cwsInstrumentationPodAnotationStatus] == cwsInstrumentationPodAnotationReady
}

// CWSInstrumentation is the main handler for the CWS instrumentation mutating webhook endpoints
type CWSInstrumentation struct {
	// filter is used to filter the pods to instrument
	filter *containers.Filter
	// image is the full image string used to configure the init container of the CWS instrumentation
	image string
	// resources is the resources applied to the CWS instrumentation init container
	resources *corev1.ResourceRequirements
}

// NewCWSInstrumentation parses the webhook config and returns a new instance of CWSInstrumentation
func NewCWSInstrumentation() (*CWSInstrumentation, error) {
	var ci CWSInstrumentation
	var err error

	// Parse filters
	ci.filter, err = containers.NewFilter(
		containers.GlobalFilter,
		config.Datadog.GetStringSlice("admission_controller.cws_instrumentation.include"),
		config.Datadog.GetStringSlice("admission_controller.cws_instrumentation.exclude"),
	)
	if err != nil {
		return nil, fmt.Errorf("couldn't initialize filter: %w", err)
	}

	// Parse init container image
	cwsInjectorImageName := config.Datadog.GetString("admission_controller.cws_instrumentation.image_name")
	cwsInjectorImageTag := config.Datadog.GetString("admission_controller.cws_instrumentation.image_tag")
	cwsInjectorContainerRegistry := config.Datadog.GetString("admission_controller.cws_instrumentation.container_registry")

	if len(cwsInjectorImageName) == 0 {
		return nil, fmt.Errorf("can't initialize CWS Instrumentation without an image_name")
	}
	if len(cwsInjectorImageTag) == 0 {
		cwsInjectorImageTag = "latest"
	}

	ci.image = fmt.Sprintf("%s:%s", cwsInjectorImageName, cwsInjectorImageTag)
	if len(cwsInjectorContainerRegistry) > 0 {
		ci.image = fmt.Sprintf("%s/%s", cwsInjectorContainerRegistry, ci.image)
	}

	// Parse init container resources
	ci.resources, err = parseCWSInitContainerResources()
	if err != nil {
		return nil, fmt.Errorf("couldn't parse CWS Instrumentation init container resources: %w", err)
	}
	return &ci, nil
}

// InjectCWSCommandInstrumentation injects CWS pod exec instrumentation
func (ci *CWSInstrumentation) InjectCWSCommandInstrumentation(rawPodExecOptions []byte, name string, ns string, userInfo *authenticationv1.UserInfo, dc dynamic.Interface, apiClient kubernetes.Interface) ([]byte, error) {
	return mutatePodExecOptions(rawPodExecOptions, name, ns, userInfo, ci.injectCWSCommandInstrumentation, dc, apiClient)
}

func (ci *CWSInstrumentation) injectCWSCommandInstrumentation(exec *corev1.PodExecOptions, name string, ns string, userInfo *authenticationv1.UserInfo, _ dynamic.Interface, apiClient kubernetes.Interface) error {
	var injected bool
	defer func() {
		metrics.MutationAttempts.Inc(metrics.CWSExecInstrumentation, strconv.FormatBool(injected), "", "")
	}()

	if exec == nil || userInfo == nil {
		metrics.MutationErrors.Inc(metrics.CWSExecInstrumentation, "nil exec or user info", "", "")
		return fmt.Errorf("cannot inject CWS instrumentation into nil exec options or nil userInfo")
	}
	if len(exec.Command) == 0 {
		metrics.MutationErrors.Inc(metrics.CWSExecInstrumentation, "empty command", "", "")
		return nil
	}

	// is the namespace / container targeted by the instrumentation ?
	if ci.filter.IsExcluded(nil, exec.Container, "", ns) {
		return nil
	}

	// check if the pod has been instrumented
	pod, err := apiClient.CoreV1().Pods(ns).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil || pod == nil {
		metrics.MutationErrors.Inc(metrics.CWSExecInstrumentation, "cannot get pod", "", "")
		return fmt.Errorf("couldn't describe pod %s in namespace %s from the API server: %w", name, ns, err)
	}

	// is the pod targeted by the instrumentation ?
	if ci.filter.IsExcluded(pod.Annotations, "", "", "") {
		return nil
	}

	// is the pod instrumentation ready ? (i.e. has the CWS Instrumentation pod admission controller run ?)
	if !isPodCWSInstrumentationReady(pod.Annotations) {
		// pod isn't instrumented, do not attempt to override the pod exec command
		log.Debugf("Ignoring exec request into %s, pod not instrumented yet", podString(pod))
		return nil
	}

	// prepare the user session context
	userSessionCtx, err := usersessions.PrepareK8SUserSessionContext(userInfo, cwsUserSessionDataMaxSize)
	if err != nil {
		metrics.MutationErrors.Inc(metrics.CWSExecInstrumentation, "cannot serialize user info", "", "")
		log.Debugf("ignoring instrumentation of %s: %v", podString(pod), err)
		return err
	}

	if len(exec.Command) > 7 {
		// make sure the command hasn't already been instrumented (note: it shouldn't happen)
		if exec.Command[0] == filepath.Join(cwsMountPath, "cws-instrumentation") &&
			exec.Command[1] == "inject" &&
			exec.Command[2] == "--session-type" &&
			exec.Command[3] == "k8s" &&
			exec.Command[4] == "--data" &&
			exec.Command[6] == "--" {

			if exec.Command[5] == string(userSessionCtx) {
				log.Debugf("Exec request into %s is already instrumented, ignoring", podString(pod))
				injected = true
				return nil
			}
		}
	}

	// override the command with the call to cws-instrumentation
	exec.Command = append([]string{
		filepath.Join(cwsMountPath, "cws-instrumentation"),
		"inject",
		"--session-type",
		"k8s",
		"--data",
		string(userSessionCtx),
		"--",
	}, exec.Command...)

	log.Debugf("Pod exec request to %s is now instrumented for CWS", podString(pod))
	injected = true

	return nil
}

// InjectCWSPodInstrumentation injects CWS pod instrumentation
func (ci *CWSInstrumentation) InjectCWSPodInstrumentation(rawPod []byte, _ string, ns string, _ *authenticationv1.UserInfo, dc dynamic.Interface, _ kubernetes.Interface) ([]byte, error) {
	return Mutate(rawPod, ns, ci.injectCWSPodInstrumentation, dc)
}

func (ci *CWSInstrumentation) injectCWSPodInstrumentation(pod *corev1.Pod, ns string, _ dynamic.Interface) error {
	var injected bool
	defer func() {
		metrics.MutationAttempts.Inc(metrics.CWSPodInstrumentation, strconv.FormatBool(injected), "", "")
	}()

	if pod == nil {
		metrics.MutationErrors.Inc(metrics.CWSPodInstrumentation, "nil pod", "", "")
		return fmt.Errorf("cannot inject CWS instrumentation into nil pod")
	}

	// is the pod targeted by the instrumentation ?
	if ci.filter.IsExcluded(pod.Annotations, "", "", ns) {
		return nil
	}

	// check if the pod has already been instrumented
	if isPodCWSInstrumentationReady(pod.Annotations) {
		injected = true
		// nothing to do, return
		return nil
	}

	// create a new volume that will be used to share cws-instrumentation across the containers of this pod
	injectCWSVolume(pod)

	// bind mount the volume to all the containers of the pod
	for i := range pod.Spec.Containers {
		injectCWSVolumeMount(&pod.Spec.Containers[i])
	}

	// add init container to copy cws-instrumentation in the cws volume
	injectCWSInitContainer(pod, ci.resources, ci.image)

	// add label to indicate that the pod has been instrumented
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[cwsInstrumentationPodAnotationStatus] = cwsInstrumentationPodAnotationReady
	injected = true
	log.Debugf("Pod %s is now instrumented for CWS", podString(pod))
	return nil
}

func injectCWSVolume(pod *corev1.Pod) {
	volumeSource := corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{},
	}

	// make sure that the cws volume doesn't already exists
	for i, vol := range pod.Spec.Volumes {
		if vol.Name == cwsVolumeName {
			// The volume exists but does it have the expected configuration ? Override just to be sure
			pod.Spec.Volumes[i].VolumeSource = volumeSource
			return
		}
	}

	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
		Name:         cwsVolumeName,
		VolumeSource: volumeSource,
	})
}

func injectCWSVolumeMount(container *corev1.Container) {
	// make sure that the volume mount doesn't already exist
	for i, mnt := range container.VolumeMounts {
		if mnt.Name == cwsVolumeName {
			// The volume mount exists but does it have the expected configuration ? Override just to be sure
			container.VolumeMounts[i].MountPath = cwsMountPath
			return
		}
	}

	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      cwsVolumeName,
		MountPath: cwsMountPath,
	})
}

func injectCWSInitContainer(pod *corev1.Pod, resources *corev1.ResourceRequirements, image string) {
	// check if the init container has already been added
	for _, c := range pod.Spec.InitContainers {
		if c.Name == cwsInjectorInitContainerName {
			// return now, the init container has already been added
			return
		}
	}

	initContainer := corev1.Container{
		Name:    cwsInjectorInitContainerName,
		Image:   image,
		Command: []string{"/cws-instrumentation", "setup", "--cws-volume-mount", cwsMountPath},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      cwsVolumeName,
				MountPath: cwsMountPath,
			},
		},
	}
	if resources != nil {
		initContainer.Resources = *resources
	}
	pod.Spec.InitContainers = append([]corev1.Container{initContainer}, pod.Spec.InitContainers...)
}
