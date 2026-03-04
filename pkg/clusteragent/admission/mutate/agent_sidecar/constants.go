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
	agentConfigVolumeName   = "agent-config"
	agentOptionsVolumeName  = "agent-option"
	agentTmpVolumeName      = "agent-tmp"
	agentLogsVolumeName     = "agent-log"
	clusterCACertVolumeName = "agent-ca-cert"
)

const (
	// configMapCAName is the name of the ConfigMap containing the cluster agent CA certificate
	configMapCAName = "datadog-ca-cert"
	// caCertDirPath is the path to the directory containing the CA certificate
	caCertDirPath = "/etc/datadog-agent/certificates"
)

var readOnlyRootFilesystemVolumes = []corev1.Volume{
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
}

var kubernetesAPILoggingVolumes = []corev1.Volume{
	{
		Name: agentOptionsVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	},
}

var readOnlyRootFilesystemVolumeMounts = []corev1.VolumeMount{
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
}

var kubernetesAPILoggingVolumeMounts = []corev1.VolumeMount{
	{
		Name:      agentOptionsVolumeName,
		MountPath: "/opt/datadog-agent/run",
	},
}

var clusterCACertVolume = corev1.Volume{
	Name: clusterCACertVolumeName,
	VolumeSource: corev1.VolumeSource{
		ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: configMapCAName,
			},
		},
	},
}

var clusterCACertVolumeMount = corev1.VolumeMount{
	Name:      clusterCACertVolumeName,
	MountPath: caCertDirPath,
	ReadOnly:  true,
}
