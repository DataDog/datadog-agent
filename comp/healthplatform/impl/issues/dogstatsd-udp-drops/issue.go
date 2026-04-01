// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package dogstatsdudpdrops

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

// Issue provides the issue template for DogStatsD UDP buffer undersized detection
type Issue struct{}

// NewIssue creates a new DogStatsD UDP drops issue template
func NewIssue() *Issue {
	return &Issue{}
}

// BuildIssue creates a complete issue with metadata and remediation steps
func (t *Issue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	currentRcvbuf := context["current_rcvbuf"]
	if currentRcvbuf == "" {
		currentRcvbuf = "0"
	}

	recommended := context["recommended"]
	if recommended == "" {
		recommended = "25165824"
	}

	issueExtra, err := structpb.NewStruct(map[string]any{
		"current_rcvbuf": currentRcvbuf,
		"recommended":    recommended,
		"impact":         "Custom metrics may be silently dropped under high throughput without any error message",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue extra: %v", err)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "dogstatsd_udp_buffer_undersized",
		Title:       "DogStatsD UDP Receive Buffer May Be Too Small",
		Description: "The dogstatsd_so_rcvbuf setting is 0, meaning the OS chooses the UDP receive buffer size. In high-throughput environments the OS default is often too small, causing the kernel to silently drop incoming DogStatsD packets. Custom metrics are lost without any error message or log entry.",
		Category:    "configuration",
		Location:    "dogstatsd",
		Severity:    "medium",
		DetectedAt:  "", // Will be filled by health platform
		Source:      "dogstatsd",
		Extra:       issueExtra,
		Remediation: buildRemediation(),
		Tags:        []string{"dogstatsd", "udp", "packet-drops", "buffer", "configuration"},
	}, nil
}

// buildRemediation creates remediation steps for the buffer configuration issue
func buildRemediation() *healthplatform.Remediation {
	return &healthplatform.Remediation{
		Summary: "Set dogstatsd_so_rcvbuf to a larger value (e.g. 25165824 = 24 MB) in datadog.yaml and restart the agent",
		Steps: []*healthplatform.RemediationStep{
			{Order: 1, Text: "Open /etc/datadog-agent/datadog.yaml in a text editor"},
			{Order: 2, Text: "Add or update the following setting: dogstatsd_so_rcvbuf: 25165824"},
			{Order: 3, Text: "Restart the Datadog Agent: sudo systemctl restart datadog-agent"},
			{Order: 4, Text: "Verify the setting is applied by checking agent logs for 'dogstatsd_so_rcvbuf'"},
			{Order: 5, Text: "Optional: increase the OS-level maximum with: sudo sysctl -w net.core.rmem_max=26214400"},
			{Order: 6, Text: "To persist the OS setting across reboots, add net.core.rmem_max=26214400 to /etc/sysctl.conf"},
		},
	}
}
