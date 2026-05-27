// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package pcap contains e2e tests for the Remote PCAP PAR bundle.
package pcap

import _ "embed"

const (
	// pcapActionFQN is the fully-qualified action name for the Remote PCAP bundle.
	pcapActionFQN = "com.datadoghq.remoteaction.pcap.runCapture"

	// pcapBundleID is the bundle portion of pcapActionFQN (everything before the final ".").
	pcapBundleID = "com.datadoghq.remoteaction.pcap"

	// parContainerName is the name of the PAR sidecar container inside the agent DaemonSet pod.
	parContainerName = "private-action-runner"

	// agentNamespace is the Kubernetes namespace where the Datadog agent is deployed.
	agentNamespace = "datadog"

	// defaultCaptureInterface is the network interface used for PCAP captures in tests.
	// "any" captures on all interfaces available inside the container.
	defaultCaptureInterface = "any"

	// defaultCaptureDurationSecs is the packet capture duration sent to the action.
	// Must be shorter than taskTimeoutSeconds (180) set in the Helm values.
	defaultCaptureDurationSecs = 30

	// defaultCaptureFilter is a BPF filter applied during capture to bound output size.
	defaultCaptureFilter = "tcp"

	// minHelmChartVersion is the earliest Datadog chart release that includes PAR sidecar
	// support with the PCAP action bundle.  Drop this override once the framework default
	// is bumped to at least this version.
	minHelmChartVersion = "3.197.2"
)

// pcapHelmValues is the Helm values YAML embedded from config/pcap-helm-values.yaml.
//
//go:embed config/pcap-helm-values.yaml
var pcapHelmValues string
