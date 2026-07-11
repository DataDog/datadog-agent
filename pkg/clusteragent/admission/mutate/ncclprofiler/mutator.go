// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package ncclprofiler

import (
	corev1 "k8s.io/api/core/v1"

	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
)

const (
	// soVolumeName is the emptyDir volume name for the injected .so files.
	soVolumeName = "datadog-nccl-profiler"

	// soMountPath is the directory where the .so volume is mounted.
	soMountPath = "/datadog-nccl"

	// soDestPath is the full in-container path to the Inspector .so after injection.
	// NCCL_PROFILER_PLUGIN points here; NCCL dlopens this and the patched Inspector
	// emits events directly to the agent's Unix socket (no wrapper layer).
	//
	// Why a full path (and not a basename + LD_LIBRARY_PATH):
	// - This matches NVIDIA's documented K8s deployment pattern for NCCL
	//   Inspector. Sending a full path lets NCCL's loader dlopen it directly
	//   without consulting LD_LIBRARY_PATH, which means we never override
	//   image-defined LD_LIBRARY_PATH (e.g. nvidia/cuda's /usr/local/nvidia/lib).
	// - Works on every NCCL version with a profiler API (2.23+) EXCEPT NCCL
	//   2.27.3, which has an upstream loader bug (NVIDIA/nccl#1732) that
	//   rejects path-style values. NCCL 2.27.3 ships only with torch 2.8.x;
	//   fixed in NCCL 2.27.5 (torch 2.9.0+).
	//
	// Customer workarounds for torch 2.8.x (NCCL 2.27.3):
	//   A. (recommended) Upgrade just NCCL: `pip install nvidia-nccl-cu12==2.27.5`.
	//      Torch dlopens NCCL at runtime, so the wheel upgrade is transparent.
	//   B. Upgrade to torch 2.9.0+ (bundles NCCL 2.27.5+).
	//   C. Override the env vars in the PodSpec. NCCL_PROFILER_PLUGIN must be
	//      the basename, and LD_LIBRARY_PATH must include /datadog-nccl plus
	//      every directory the image's Dockerfile sets (read out of the
	//      image with: `docker run --rm <image> sh -c 'echo $LD_LIBRARY_PATH'`):
	//        env:
	//        - name: NCCL_PROFILER_PLUGIN
	//          value: "libnccl-profiler-inspector.so"
	//        - name: LD_LIBRARY_PATH
	//          value: "/datadog-nccl:/usr/local/nvidia/lib:/usr/local/nvidia/lib64"
	//      Setting only "/datadog-nccl" here would drop the image's CUDA / NVIDIA
	//      lib directories — Kubernetes PodSpec env overrides image env and
	//      $LD_LIBRARY_PATH is not expanded against image vars.
	soDestPath = "/datadog-nccl/libnccl-profiler-inspector.so"

	// soSourcePathInspector is where the Inspector .so lives inside the injector image.
	soSourcePathInspector = "/libnccl-profiler-inspector.so"

	// socketVolumeName is the hostPath volume name for the Datadog agent socket.
	socketVolumeName = "datadog-socket"
)

// mutatePod injects the NCCL profiler plugin into pod by:
//  1. Adding an emptyDir volume for the Inspector .so.
//  2. Prepending an init container that copies the Inspector .so from the injector image.
//  3. Mounting the .so volume and the agent socket directory into every app container.
//  4. Setting NCCL env vars (incl. NCCL_DD_SOCKET_PATH from the agent config).
//
// Mounts the agent socket DIRECTORY (HostPathDirectoryOrCreate) and points
// NCCL_DD_SOCKET_PATH at the socket file inside it. Mounting the directory --
// not the file via HostPathSocket -- keeps the path valid across agent
// restarts: the agent re-creates the socket with a new inode on every start
// (os.Remove + ListenUnix), so a file bind-mount would strand long-lived
// workload pods on the dead old inode (agent then receives 0 events).
// initResources is the optional resource requirements applied to the injected
// init container. nil means no Resources block is set (cluster default applies);
// operators with a LimitRange or strict QoS requirements override via
// admission_controller.nccl_profiler.init_resources.{cpu,memory}.
//
// Pod-level opt-in policy (label + mutate_unlabelled) is enforced by the
// webhook objectSelector at the K8s API server, not re-checked here.
func mutatePod(pod *corev1.Pod, injectorImage, hostSocketDir, clientSocketDir, socketFilename string, initResources *corev1.ResourceRequirements) (bool, error) {
	soVolume := corev1.Volume{
		Name:         soVolumeName,
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}
	soMount := corev1.VolumeMount{Name: soVolumeName, MountPath: soMountPath, ReadOnly: true}

	// Pod paths are POSIX regardless of where the cluster-agent runs; do not use
	// path/filepath here (it'd produce backslashes on Windows).
	clientFile := clientSocketDir + "/" + socketFilename
	// Mount the DIRECTORY, not the socket file: a HostPathSocket (file) bind-mount
	// pins the inode at pod-creation time, and the agent recreates the socket on
	// every restart, stranding the pod on a dead inode. Mounting the directory
	// lets the in-pod NCCL_DD_SOCKET_PATH re-resolve to the live socket each time.
	hostPathType := corev1.HostPathDirectoryOrCreate
	volume := corev1.Volume{
		Name: socketVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{Path: hostSocketDir, Type: &hostPathType},
		},
	}
	volumeMount := corev1.VolumeMount{Name: socketVolumeName, MountPath: clientSocketDir, ReadOnly: true}

	// Inject volumes + mounts into all app containers using shared helpers.
	soVolAdded, soMountAdded := mutatecommon.InjectVolume(pod, soVolume, soMount)
	sockVolAdded, sockMountAdded := mutatecommon.InjectVolume(pod, volume, volumeMount)

	// Inject NCCL env vars into all app containers.
	envAdded := mutatecommon.InjectEnv(pod, corev1.EnvVar{Name: "NCCL_PROFILER_PLUGIN", Value: soDestPath})
	envAdded = mutatecommon.InjectEnv(pod, corev1.EnvVar{Name: "NCCL_DD_SOCKET_PATH", Value: clientFile}) || envAdded
	envAdded = mutatecommon.InjectEnv(pod, corev1.EnvVar{Name: "NCCL_DD_INSPECTOR_PATH", Value: soMountPath + "/libnccl-profiler-inspector.so"}) || envAdded
	envAdded = mutatecommon.InjectEnv(pod, corev1.EnvVar{Name: "NCCL_INSPECTOR_ENABLE", Value: "1"}) || envAdded

	// Point NVIDIA Inspector's file-dump at /tmp — writable in virtually
	// every customer training container. Without this, Inspector's dump
	// thread tries the default location, and if THAT isn't writable, the
	// thread breaks before calling our patched inspectorCommInfoDump →
	// socket delivery silently emits nothing. Customer can override.
	envAdded = mutatecommon.InjectEnv(pod, corev1.EnvVar{Name: "NCCL_INSPECTOR_DUMP_DIR", Value: "/tmp/nccl-inspector"}) || envAdded

	// Inspector's dump thread sets the needs_writing flag that gates the
	// .so's socket-delivery hook. Default interval is 0 (thread sits
	// idle), so without an explicit value the .so emits nothing. Default
	// to 100 ms; consumers can override (InjectEnv is a no-op if already
	// set). The Inspector .so also setenv-defaults this at init time as a
	// belt-and-suspenders for pods loading the .so outside the webhook.
	envAdded = mutatecommon.InjectEnv(pod, corev1.EnvVar{Name: "NCCL_INSPECTOR_DUMP_THREAD_INTERVAL_MICROSECONDS", Value: "100000"}) || envAdded

	// Prepend init container that copies the Inspector .so from the injector image.
	// SecurityContext drops all capabilities + disallows privilege escalation so
	// the container passes the "restricted" PodSecurity standard. Resource
	// requirements are operator-supplied (nil = cluster default applies).
	allowPrivEsc := false
	initContainer := corev1.Container{
		Name:            "datadog-nccl-profiler-inject",
		Image:           injectorImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"sh", "-c", "cp " + soSourcePathInspector + " " + soMountPath + "/"},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &allowPrivEsc,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: soVolumeName, MountPath: soMountPath}, // writable for init container
		},
	}
	if initResources != nil {
		initContainer.Resources = *initResources
	}
	alreadyInjected := false
	for _, c := range pod.Spec.InitContainers {
		if c.Name == initContainer.Name {
			alreadyInjected = true
			break
		}
	}
	initAdded := false
	if !alreadyInjected {
		pod.Spec.InitContainers = append([]corev1.Container{initContainer}, pod.Spec.InitContainers...)
		initAdded = true
	}

	mutated := soVolAdded || soMountAdded || sockVolAdded || sockMountAdded || envAdded || initAdded
	return mutated, nil
}
