// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infra

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatProgressLine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		out  string
		show bool
	}{
		{
			name: "resource-creating",
			in:   " +  aws:eks:Cluster myeks creating...",
			out:  "  ⏳ aws:eks:Cluster myeks creating...",
			show: true,
		},
		{
			name: "resource-created",
			in:   " +  aws:eks:Cluster myeks created (45s)",
			out:  "  ✓ aws:eks:Cluster myeks created (45s)",
			show: true,
		},
		{
			name: "resource-updating",
			in:   " ~  aws:ec2:SecurityGroup myeks-sg updating...",
			out:  "  ⏳ aws:ec2:SecurityGroup myeks-sg updating...",
			show: true,
		},
		{
			name: "resource-updated",
			in:   " ~  aws:ec2:SecurityGroup myeks-sg updated (1m10s)",
			out:  "  ✓ aws:ec2:SecurityGroup myeks-sg updated (1m10s)",
			show: true,
		},
		{
			name: "resource-deleting",
			in:   " -  aws:s3:Bucket my-bucket deleting...",
			out:  "  ⏳ aws:s3:Bucket my-bucket deleting...",
			show: true,
		},
		{
			name: "resource-deleted",
			in:   " -  aws:s3:Bucket my-bucket deleted (3s)",
			out:  "  ✓ aws:s3:Bucket my-bucket deleted (3s)",
			show: true,
		},
		{
			name: "resource-read",
			in:   " >  aws:ec2:Instance my-ami read (2s)",
			out:  "  ✓ aws:ec2:Instance my-ami read (2s)",
			show: true,
		},
		{
			name: "stack-lifecycle-line",
			in:   " +  pulumi:pulumi:Stack my-stack creating...",
			out:  "  ⏳ pulumi:pulumi:Stack my-stack creating...",
			show: true,
		},
		{
			name: "resource-failed-with-error-details",
			in:   " +  aws:iam:Role myeks-role **creating failed** error: API rate limit exceeded",
			out:  "  ✗ aws:iam:Role myeks-role failed: API rate limit exceeded",
			show: true,
		},
		{
			name: "resource-failed-without-error-details",
			in:   " ~  aws:iam:Role myeks-role **updating failed**",
			out:  "  ✗ aws:iam:Role myeks-role failed",
			show: true,
		},
		{
			name: "unknown-verb-is-preserved",
			in:   " +  aws:ec2:Instance my-instance configuring special-mode",
			out:  "  ? aws:ec2:Instance my-instance configuring special-mode",
			show: true,
		},
		{name: "empty-line", in: "", show: false},
		{name: "whitespace-only-line", in: "    ", show: false},
		{name: "provider-read-is-internal", in: " >  pulumi:providers:aws default_6_0_0 read (0.001s)", show: false},
		{name: "summary-resources", in: "Resources:", show: false},
		{name: "summary-count", in: "    + 23 created", show: false},
		{name: "summary-duration", in: "Duration: 10s", show: false},
		{name: "engine-noise", in: "loading policy pack", show: false},
		{
			name: "diagnostic-error",
			in:   "    error: update failed",
			out:  "error: update failed",
			show: true,
		},
		{
			name: "diagnostic-warning",
			in:   "    warning: deprecated field",
			out:  "warning: deprecated field",
			show: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, show := formatProgressLine(tc.in)
			assert.Equal(t, tc.show, show)
			if tc.show {
				assert.Equal(t, tc.out, out)
			}
		})
	}
}

func TestProgressFilterWriter(t *testing.T) {
	var buf bytes.Buffer
	filter := NewProgressFilter(&buf)

	// First chunk ends in the middle of a line: nothing must be emitted yet.
	raw := " +  aws:eks:Cluster myeks creat"
	n, err := fmt.Fprint(filter, raw)
	require.NoError(t, err)
	require.Equal(t, len(raw), n)
	assert.Empty(t, buf.String())

	// Remaining chunks: one complete line, then noise, then a second line split
	// across two writes.
	raw = "ing...\n\nResources:\n    + 23 created\n ~  aws:ec2:SecurityGroup myeks-sg up"
	_, err = fmt.Fprint(filter, raw)
	require.NoError(t, err)
	assert.Equal(t, "  ⏳ aws:eks:Cluster myeks creating...\n", buf.String())

	raw = "dating...\n +  aws:iam:Role myeks-role **creating failed** error: rate limited\n"
	_, err = fmt.Fprint(filter, raw)
	require.NoError(t, err)

	assert.Equal(t, ""+
		"  ⏳ aws:eks:Cluster myeks creating...\n"+
		"  ⏳ aws:ec2:SecurityGroup myeks-sg updating...\n"+
		"  ✗ aws:iam:Role myeks-role failed: rate limited\n",
		buf.String())
}
