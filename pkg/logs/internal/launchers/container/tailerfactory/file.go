// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package tailerfactory

// This file handles creating docker tailers which access the container runtime
// via files.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/container/tailerfactory/tailers"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/status"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/containersorpods"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	dockerutilPkg "github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

var podLogsBasePath = "/var/log/pods"
var dockerLogsBasePathNix = "/var/lib/docker"
var dockerLogsBasePathWin = "c:\\programdata\\docker"
var podmanLogsBasePath = "/var/lib/containers"

// makeFileTailer makes a file-based tailer for the given source, or returns
// an error if it cannot do so (e.g., due to permission errors)
func (tf *factory) makeFileTailer(source *sources.LogSource) (Tailer, error) {
	fileSource, err := tf.makeFileSource(source)
	if err != nil {
		return nil, err
	}
	return tf.attachChildSource(source, fileSource)
}

// makeFileSource makes a new LogSource with Config.Type=="file" to log the
// given source.
func (tf *factory) makeFileSource(source *sources.LogSource) (*sources.LogSource, error) {
	// The user configuration consulted is different depending on what we are
	// logging.  Note that we assume that by the time we have gotten a source
	// from AD, it is clear what we are logging.  The `Wait` here should return
	// quickly.
	logWhat := tf.cop.Wait(context.Background())

	switch logWhat {
	case containersorpods.LogContainers:
		// determine what the type of file source to create depending on the
		// container runtime
		switch source.Config.Type {
		case "docker":
			return tf.makeDockerFileSource(source)
		default:
			return nil, fmt.Errorf("file tailing is not supported for source type %s", source.Config.Type)
		}

	case containersorpods.LogPods:
		return tf.makeK8sFileSource(source)

	default:
		// if this occurs, then sources have been arriving before the
		// container interfaces to them are ready.  Something is wrong.
		return nil, fmt.Errorf("LogWhat = %s; not ready to log containers", logWhat.String())
	}
}

// attachChildSource attaches a child source to the parent source, sorting out
// status, info, and so on.
func (tf *factory) attachChildSource(source, childSource *sources.LogSource) (Tailer, error) {
	containerID := source.Config.Identifier

	sourceInfo := status.NewMappedInfo("Container Info")
	source.RegisterInfo(sourceInfo)

	// Update parent source with additional information
	sourceInfo.SetMessage(containerID,
		fmt.Sprintf("Container ID: %s, Tailing from file: %s",
			dockerutilPkg.ShortContainerID(containerID),
			childSource.Config.Path))

	// link status for this source and the parent, and hide the parent
	childSource.Status = source.Status
	childSource.ParentSource = source
	source.HideFromStatus()

	// return a "tailer" that will schedule and unschedule this source
	// when started and stopped
	return &tailers.WrappedSource{
		Source:  childSource,
		Sources: tf.sources,
	}, nil
}

// makeDockerFileSource makes a LogSource with Config.Type="file" for a docker container.
func (tf *factory) makeDockerFileSource(source *sources.LogSource) (*sources.LogSource, error) {
	containerID := source.Config.Identifier

	path := tf.findDockerLogPath(containerID)

	// check access to the file; if it is not readable, then returning an error will
	// try to fall back to reading from a socket.
	f, err := filesystem.OpenShared(path)
	if err != nil {
		// (this error already has the form 'open <path>: ..' so needs no further embellishment)
		return nil, err
	}
	f.Close()

	sourceName, serviceName := tf.defaultSourceAndService(source, containersorpods.LogContainers)

	// New file source that inherits most of its parent's properties
	fileSource := sources.NewLogSource(source.Name, &config.LogsConfig{
		Type:            config.FileType,
		Identifier:      containerID,
		Path:            path,
		Service:         serviceName,
		Source:          sourceName,
		Tags:            source.Config.Tags,
		ProcessingRules: source.Config.ProcessingRules,
	})

	// inform the file launcher that it should expect docker-formatted content
	// in this file
	fileSource.SetSourceType(sources.DockerSourceType)

	return fileSource, nil
}

// findDockerLogPath returns a path for the given container.
func (tf *factory) findDockerLogPath(containerID string) string {
	// if the user has set a custom docker data root, this will pick it up
	// and set it in place of the usual docker base path
	overridePath := coreConfig.Datadog.GetString("logs_config.docker_path_override")
	if len(overridePath) > 0 {
		return filepath.Join(overridePath, "containers", containerID, fmt.Sprintf("%s-json.log", containerID))
	}

	switch runtime.GOOS {
	case "windows":
		return filepath.Join(
			dockerLogsBasePathWin, "containers", containerID,
			fmt.Sprintf("%s-json.log", containerID))
	default: // linux, darwin
		// this config flag provides temporary support for podman while it is
		// still recognized by AD as a "docker" runtime.
		if coreConfig.Datadog.GetBool("logs_config.use_podman_logs") {
			return filepath.Join(
				podmanLogsBasePath, "storage/overlay-containers", containerID,
				"userdata/ctr.log")
		}
		return filepath.Join(
			dockerLogsBasePathNix, "containers", containerID,
			fmt.Sprintf("%s-json.log", containerID))
	}
}

// makeK8sFileSource makes a LogSource with Config.Type="file" for a container in a K8s pod.
func (tf *factory) makeK8sFileSource(source *sources.LogSource) (*sources.LogSource, error) {
	containerID := source.Config.Identifier

	pod, err := tf.workloadmetaStore.GetKubernetesPodForContainer(containerID)
	if err != nil {
		return nil, fmt.Errorf("cannot find pod for container %q: %w", containerID, err)
	}

	var container *workloadmeta.OrchestratorContainer
	for _, pc := range pod.Containers {
		if pc.ID == containerID {
			container = &pc
			break
		}
	}

	if container == nil {
		// this failure is impossible, as GetKubernetesPodForContainer found
		// the pod by searching for this container
		return nil, fmt.Errorf("cannot find container %q in pod %q", containerID, pod.Name)
	}

	// get the path for the discovered pod and container
	// TODO: need a different base path on windows?
	path := findK8sLogPath(pod, container.Name)

	// Note that it's not clear from k8s documentation that the container logs,
	// or even the directory containing these logs, must exist at this point.
	// To avoid incorrectly falling back to socket logging (or failing to log
	// entirely) we do not check for the file here. This matches older
	// kubernetes-launcher behavior.

	sourceName, serviceName := tf.defaultSourceAndService(source, containersorpods.LogPods)

	// New file source that inherits most of its parent's properties
	fileSource := sources.NewLogSource(
		fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, container.Name),
		&config.LogsConfig{
			Type:            config.FileType,
			Identifier:      containerID,
			Path:            path,
			Service:         serviceName,
			Source:          sourceName,
			Tags:            source.Config.Tags,
			ProcessingRules: source.Config.ProcessingRules,
		})

	switch source.Config.Type {
	case config.DockerType:
		// docker runtime uses SourceType "docker"
		fileSource.SetSourceType(sources.DockerSourceType)
	default:
		// containerd runtime uses SourceType "kubernetes"
		fileSource.SetSourceType(sources.KubernetesSourceType)
	}

	return fileSource, nil
}

// findK8sLogPath returns a wildcard matching logs files for the given container in pod.
func findK8sLogPath(pod *workloadmeta.KubernetesPod, containerName string) string {
	// the pattern for container logs is different depending on the version of Kubernetes
	// so we need to try three possbile formats
	// until v1.9 it was `/var/log/pods/{pod_uid}/{container_name_n}.log`,
	// v.1.10 to v1.13 it was `/var/log/pods/{pod_uid}/{container_name}/{n}.log`,
	// since v1.14 it is `/var/log/pods/{pod_namespace}_{pod_name}_{pod_uid}/{container_name}/{n}.log`.
	// see: https://github.com/kubernetes/kubernetes/pull/74441 for more information.

	const (
		anyLogFile    = "*.log"
		anyV19LogFile = "%s_*.log"
	)

	// getPodDirectoryUntil1_13 returns the name of the directory of pod containers until Kubernetes v1.13.
	getPodDirectoryUntil1_13 := func(pod *workloadmeta.KubernetesPod) string {
		return pod.ID
	}

	// getPodDirectorySince1_14 returns the name of the directory of pod containers since Kubernetes v1.14.
	getPodDirectorySince1_14 := func(pod *workloadmeta.KubernetesPod) string {
		return fmt.Sprintf("%s_%s_%s", pod.Namespace, pod.Name, pod.ID)
	}

	oldDirectory := filepath.Join(podLogsBasePath, getPodDirectoryUntil1_13(pod))
	if _, err := os.Stat(oldDirectory); err == nil {
		v110Dir := filepath.Join(oldDirectory, containerName)
		_, err := os.Stat(v110Dir)
		if err == nil {
			log.Debugf("Logs path found for container %s, v1.13 >= kubernetes version >= v1.10", containerName)
			return filepath.Join(v110Dir, anyLogFile)
		}
		if !os.IsNotExist(err) {
			log.Debugf("Cannot get file info for %s: %v", v110Dir, err)
		}

		v19Files := filepath.Join(oldDirectory, fmt.Sprintf(anyV19LogFile, containerName))
		files, err := filepath.Glob(v19Files)
		if err == nil && len(files) > 0 {
			log.Debugf("Logs path found for container %s, kubernetes version <= v1.9", containerName)
			return v19Files
		}
		if err != nil {
			log.Debugf("Cannot get file info for %s: %v", v19Files, err)
		}
		if len(files) == 0 {
			log.Debugf("Files matching %s not found", v19Files)
		}
	}

	log.Debugf("Using the latest kubernetes logs path for container %s", containerName)
	return filepath.Join(podLogsBasePath, getPodDirectorySince1_14(pod), containerName, anyLogFile)
}
