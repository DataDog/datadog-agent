// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gpupodresources

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	contextKeyError      = "error"
	contextKeySocketPath = "socket_path"

	unknownValue = "unknown"
)

// GPUPodResourcesIssue builds an Agent Health issue for an inaccessible
// Kubernetes PodResources API.
type GPUPodResourcesIssue struct{}

// BuildIssue creates a complete issue with remediation for PodResources API failures.
func (GPUPodResourcesIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	socketPath := context[contextKeySocketPath]
	if socketPath == "" {
		socketPath = unknownValue
	}
	errMsg := context[contextKeyError]
	if errMsg == "" {
		errMsg = unknownValue
	}

	extra, err := structpb.NewStruct(map[string]any{
		contextKeyError:      errMsg,
		contextKeySocketPath: socketPath,
		"impact":             "GPU workload attribution may be unavailable",
	})
	if err != nil {
		return nil, fmt.Errorf("create issue extra: %w", err)
	}

	return &healthplatform.Issue{
		IssueName:   IssueName,
		IssueType:   IssueType,
		Title:       fmt.Sprintf("GPU monitoring cannot access PodResources API at %q", socketPath),
		Description: fmt.Sprintf("The Datadog Agent cannot access the Kubernetes PodResources API at %q: %s. GPU workload attribution may be unavailable.", socketPath, errMsg),
		Category:    "availability",
		Location:    "gpu",
		Severity:    healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
		Source:      "gpu",
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Make the kubelet PodResources socket accessible to the Datadog Agent.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Check the following error returned by PodResources API: " + errMsg},
				{Order: 2, Text: fmt.Sprintf("Verify that the kubelet PodResources socket exists on the node at `%s`.", socketPath)},
				{Order: 3, Text: "Verify that the Agent DaemonSet mounts the PodResources socket path and that the Agent container can read and connect to it."},
				{Order: 4, Text: "Check the `kubernetes_kubelet_podresources_socket` setting if the kubelet uses a non-default socket path."},
				{Order: 5, Text: "Review GPU monitoring setup: https://docs.datadoghq.com/gpu_monitoring/setup/"},
			},
		},
		Tags: []string{"gpu", "kubernetes", "podresources"},
	}, nil
}
