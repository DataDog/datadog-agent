// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
)

func TestGetCloudServiceType(t *testing.T) {
	os.Setenv(ContainerAppNameEnvVar, "test-name")
	assert.Equal(t, GetCloudServiceType(), &ContainerApp{})

	os.Unsetenv(ContainerAppNameEnvVar)
	assert.Equal(t, GetCloudServiceType(), &CloudRun{})
}

func TestFormatTraceForCloudRunWithValidRootSpan(t *testing.T) {
	spans := []*pb.Span{
		{
			SpanID:   1,
			ParentID: 123,
		},
		{
			SpanID:   2,
			ParentID: 124,
		},
		{
			SpanID:   3,
			ParentID: 0,
		},
	}

	spans = WrapSpans("gcp.cloudrun", spans)

	assert.Len(t, spans, 4)

	oldRootSpan := spans[2]
	newRootSpan := spans[3]

	assert.Equal(t, "gcp.cloudrun", newRootSpan.Name)
	assert.Equal(t, "gcp.cloudrun", newRootSpan.Resource)
	assert.Equal(t, uint64(0), newRootSpan.ParentID)
	assert.Equal(t, newRootSpan.SpanID, oldRootSpan.ParentID)
	assert.Equal(t, newRootSpan.TraceID, oldRootSpan.TraceID)
}

func TestFormatTraceForCloudRunWithNoRootSpan(t *testing.T) {
	spans := []*pb.Span{
		{
			SpanID:   1,
			ParentID: 123,
		},
		{
			SpanID:   2,
			ParentID: 124,
		},
		{
			SpanID:   3,
			ParentID: 125,
		},
	}

	spans = WrapSpans("gcp.cloudrun", spans)

	assert.Len(t, spans, 3)
}
