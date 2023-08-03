// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package mutate

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usersessions"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	cwsVolumeName                        = "datadog-cws-instrumentation"
	cwsMountPath                         = "/datadog-cws-instrumentation"
	cwsInstrumentationPodAnotationStatus = "admission.datadoghq.com/cws-instrumentation.status"
	cwsInstrumentationPodAnotationReady  = "ready"
	cwsInjectorInitContainerName         = "cws-injector"
	cwsUserSessionDataMaxSize            = 1024

	// CWSInstrumentationPodLabelEnabled is used to label pods that should be instrumented or skipped by the CWS mutating webhook
	CWSInstrumentationPodLabelEnabled = "admission.datadoghq.com/cws-instrumentation.enabled"
)

// isNsTargetedByCWSInstrumentation returns true if the provided namespace should be instrumented
func isNsTargetedByCWSInstrumentation(ns string) bool {
	// We have to fetch the config at runtime otherwise we might behave differently depending on how the configuration
	// was provided (env variable or config file).
	cwsTargetNamespaces := config.Datadog.GetStringSlice("admission_controller.cws_instrumentation.target.namespaces")
	cwsTargetAllNamespaces := config.Datadog.GetBool("admission_controller.cws_instrumentation.target.all_namespaces")

	if cwsTargetAllNamespaces {
		return true
	}
	if len(cwsTargetNamespaces) == 0 {
		return false
	}
	for _, targetNs := range cwsTargetNamespaces {
		if ns == targetNs {
			return true
		}
	}
	return false
}

// isPodCWSInstrumentationReady returns true if the pod has already been instrumented by CWS
func isPodCWSInstrumentationReady(annotations map[string]string) bool {
	return annotations[cwsInstrumentationPodAnotationStatus] == cwsInstrumentationPodAnotationReady
}

// InjectCWSCommandInstrumentation injects CWS pod exec instrumentation
func InjectCWSCommandInstrumentation(rawPodExecOptions []byte, name string, ns string, userInfo *authenticationv1.UserInfo, dc dynamic.Interface, apiClient kubernetes.Interface) ([]byte, error) {
	return mutatePodExecOptions(rawPodExecOptions, name, ns, userInfo, injectCWSCommandInstrumentation, dc, apiClient)
}

func injectCWSCommandInstrumentation(exec *corev1.PodExecOptions, name string, ns string, userInfo *authenticationv1.UserInfo, _ dynamic.Interface, apiClient kubernetes.Interface) error {
	if exec == nil || userInfo == nil {
		return fmt.Errorf("cannot inject CWS instrumentation into nil exec options or nil userInfo")
	}
	if len(exec.Command) == 0 {
		return fmt.Errorf("cannot inject CWS instrumentation into empty exec command")
	}

	// is the namespace targeted by the instrumentation ?
	if !isNsTargetedByCWSInstrumentation(ns) {
		return nil
	}

	// check if the pod has been instrumented
	pod, err := apiClient.CoreV1().Pods(ns).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("couldn't describe pod %s in namespace %s from the API server: %w", name, ns, err)
	}

	// is the pod instrumentation ready ? (i.e. has the CWS Instrumentation pod admission controller run ?)
	if !isPodCWSInstrumentationReady(pod.GetAnnotations()) {
		// pod isn't instrumented, do not attempt to override the pod exec command
		log.Debugf("Ignoring exec request into %s, pod not instrumented yet", podString(pod))
		return nil
	}

	// prepare the user session context
	userSessionCtx, err := prepareUserSessionContext(userInfo)
	if err != nil {
		log.Debugf("ignoring instrumentation of %s: %v", podString(pod), err)
		return nil
	}

	if len(exec.Command) > 7 {
		// make sure the command hasn't already been instrumented
		if exec.Command[0] == filepath.Join(cwsMountPath, "cws-injector") &&
			exec.Command[1] == "inject" &&
			exec.Command[2] == "--session-type" &&
			exec.Command[3] == "k8s" &&
			exec.Command[4] == "--data" &&
			exec.Command[6] == "--" {

			if exec.Command[5] == string(userSessionCtx) {
				log.Debugf("Exec request into %s is already instrumented, ignoring", podString(pod))
				return nil
			}
		}
	}

	// override the command with the call to cws-injector
	exec.Command = append([]string{
		filepath.Join(cwsMountPath, "cws-injector"),
		"inject",
		"--session-type",
		"k8s",
		"--data",
		string(userSessionCtx),
		"--",
	}, exec.Command...)

	log.Debugf("Pod exec request to %s is now instrumented for CWS", podString(pod))

	return nil
}

func prepareUserSessionContext(userInfo *authenticationv1.UserInfo) ([]byte, error) {
	userSessionCtx, err := json.Marshal(userInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to marshall authenticationv1.UserInfo structure: %w", err)
	}
	if len(userSessionCtx) <= cwsUserSessionDataMaxSize {
		return userSessionCtx, nil
	}

	// try to remove the extra field
	info := *userInfo
	info.Extra = nil

	userSessionCtx, err = json.Marshal(info)
	if err != nil {
		return nil, fmt.Errorf("failed to marshall authenticationv1.UserInfo structure: %w", err)
	}
	if len(userSessionCtx) <= cwsUserSessionDataMaxSize {
		return userSessionCtx, nil
	}

	// try to remove the groups field
	info.Groups = nil
	userSessionCtx, err = json.Marshal(info)
	if err != nil {
		return nil, fmt.Errorf("failed to marshall authenticationv1.UserInfo structure: %w", err)
	}

	if len(userSessionCtx) <= cwsUserSessionDataMaxSize {
		return userSessionCtx, nil
	}
	return nil, fmt.Errorf("authenticationv1.UserInfo structure too big (%d), ignoring instrumentation", len(userSessionCtx))
}

// InjectCWSPodInstrumentation injects CWS pod instrumentation
func InjectCWSPodInstrumentation(rawPod []byte, _ string, ns string, _ *authenticationv1.UserInfo, dc dynamic.Interface, _ kubernetes.Interface) ([]byte, error) {
	return mutate(rawPod, ns, injectCWSPodInstrumentation, dc)
}

func injectCWSPodInstrumentation(pod *corev1.Pod, ns string, _ dynamic.Interface) error {
	if pod == nil {
		return fmt.Errorf("cannot inject CWS instrumentation into nil pod")
	}

	// sanity check: make sure the provided CWS Injector image name isn't empty
	if len(config.Datadog.GetString("admission_controller.cws_instrumentation.cws_injector_image_name")) == 0 {
		// don't touch the pod and leave now
		return fmt.Errorf("empty CWS Injector image name")
	}

	// is the namespace targeted by the instrumentation ?
	if !isNsTargetedByCWSInstrumentation(ns) {
		return nil
	}

	annotations := pod.GetAnnotations()

	// check if the pod has already been instrumented
	if isPodCWSInstrumentationReady(annotations) {
		// nothing to do, return
		return nil
	}

	// create a new volume that will be used to share cws-injector across the containers of this pod
	injectCWSVolume(pod)

	// bind mount the volume to all the containers of the pod
	for i := range pod.Spec.Containers {
		injectCWSVolumeMount(&pod.Spec.Containers[i])
	}

	// add init container to copy cws-injector in the cws volume
	injectCWSInitContainer(pod)

	// add label to indicate that the pod has been instrumented
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[cwsInstrumentationPodAnotationStatus] = cwsInstrumentationPodAnotationReady
	pod.Annotations = annotations
	log.Debugf("Pod %s is now instrumented for CWS", podString(pod))
	return nil
}

func injectCWSVolume(pod *corev1.Pod) {
	// make sure that the cws volume doesn't already exists
	for _, vol := range pod.Spec.Volumes {
		if vol.Name == cwsVolumeName {
			// return now, the volume is already present
			return
		}
	}

	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
		Name: cwsVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})
}

func injectCWSVolumeMount(container *corev1.Container) {
	// make sure that the volume mount doesn't already exist
	for _, mnt := range container.VolumeMounts {
		if mnt.Name == cwsVolumeName {
			// return now, the volume mount has already been added
			return
		}
	}

	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      cwsVolumeName,
		MountPath: cwsMountPath,
	})
}

func injectCWSInitContainer(pod *corev1.Pod) {
	// check if the init container has already been added
	for _, c := range pod.Spec.InitContainers {
		if c.Name == cwsInjectorInitContainerName {
			// return now, the init container has already been added
			return
		}
	}

	// We have to fetch the config at runtime otherwise we might behave differently depending on how the configuration
	// was provided (env variable or config file).
	cwsInjectorImageName := config.Datadog.GetString("admission_controller.cws_instrumentation.cws_injector_image_name")
	cwsInjectorImageTag := config.Datadog.GetString("admission_controller.cws_instrumentation.cws_injector_image_tag")
	cwsInjectorContainerRegistry := config.Datadog.GetString("admission_controller.cws_instrumentation.cws_injector_container_registry")

	if len(cwsInjectorImageTag) == 0 {
		cwsInjectorImageTag = "latest"
	}

	var image string
	image = fmt.Sprintf("%s:%s", cwsInjectorImageName, cwsInjectorImageTag)
	if len(cwsInjectorContainerRegistry) > 0 {
		image = fmt.Sprintf("%s/%s", cwsInjectorContainerRegistry, image)
	}

	initContainer := corev1.Container{
		Name:    cwsInjectorInitContainerName,
		Image:   image,
		Command: []string{"/cws-injector", "setup", "--cws-volume-mount", cwsMountPath},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      cwsVolumeName,
				MountPath: cwsMountPath,
			},
		},
	}
	pod.Spec.InitContainers = append([]corev1.Container{initContainer}, pod.Spec.InitContainers...)
}
