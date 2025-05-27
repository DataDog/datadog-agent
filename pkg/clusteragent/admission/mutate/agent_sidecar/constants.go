// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import corev1 "k8s.io/api/core/v1"

const (
	agentSidecarContainerName = "datadog-agent-injected"
	providerFargate           = "fargate"
)

const (
	agentConfigVolumeName  = "agent-config"
	agentOptionsVolumeName = "agent-option"
	agentTmpVolumeName     = "agent-tmp"
	agentLogsVolumeName    = "agent-log"
)

type feature string

const (
	readOnlyRootFilesystemFeature feature = "read-only-root-filesystem"
	eksFargateLoggingFeature      feature = "eks-fargate-logging"
	clusterAgentFeature           feature = "cluster-agent"
)

var featuresToVolumes = map[feature][]corev1.Volume{
	readOnlyRootFilesystemFeature: {
		{
			Name: agentConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: agentOptionsVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: agentTmpVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: agentLogsVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	},
	eksFargateLoggingFeature: {
		{
			Name: agentOptionsVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	},
}

var featuresToVolumeMounts = map[feature][]corev1.VolumeMount{
	readOnlyRootFilesystemFeature: {
		{
			Name:      agentConfigVolumeName,
			MountPath: "/etc/datadog-agent",
		},
		{
			Name:      agentOptionsVolumeName,
			MountPath: "/opt/datadog-agent/run",
		},
		{
			Name:      agentTmpVolumeName,
			MountPath: "/tmp",
		},
		{
			Name:      agentLogsVolumeName,
			MountPath: "/var/log/datadog",
		},
	},
	eksFargateLoggingFeature: {
		{
			Name:      agentOptionsVolumeName,
			MountPath: "/opt/datadog-agent/run",
		},
	},
}
