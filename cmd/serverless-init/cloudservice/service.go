// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"math/rand"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

// CloudService implements getting tags from each Cloud Provider.
type CloudService interface {
	// GetTags returns a map of tags for a given cloud service. These tags are then attached to
	// the logs, traces, and metrics.
	GetTags() map[string]string

	// GetOrigin returns the value that will be used for the `origin` attribute for
	// all logs, traces, and metrics.
	GetOrigin() string

	// GetPrefix returns the prefix that we're prefixing all
	// metrics with. For example, for cloudrun, we're using
	// gcp.run.{metric_name}. In this example, `gcp.run` is the
	// prefix.
	GetPrefix() string

	// WrapSpans returns a new list of spans a new top-level span
	// coming from the associated cloud provider.
	WrapSpans([]*pb.Span) []*pb.Span
}

// WrapSpans takes a new root span name and wraps the current span list
func WrapSpans(newRootName string, spans []*pb.Span) []*pb.Span {
	oldRoot := traceutil.GetRoot(spans)
	// GetRoot returns the last span if we don't have a "true" root value.
	// If that's the case, we don't want to wrap the span.
	if oldRoot.ParentID != 0 {
		return spans
	}
	root := &pb.Span{
		TraceID:  oldRoot.TraceID,
		Name:     newRootName,
		Resource: newRootName,
		Start:    oldRoot.Start,
		SpanID:   rand.Uint64(),
		Duration: oldRoot.Duration,
		Error:    oldRoot.Error,
		Meta:     oldRoot.Meta,
		Type:     oldRoot.Type,
		Service:  oldRoot.Service,
	}
	oldRoot.ParentID = root.SpanID

	newSpans := make([]*pb.Span, len(spans))
	copy(newSpans, spans)

	return append(newSpans, root)
}

func GetCloudServiceType() CloudService {
	if isContainerAppService() {
		return &ContainerApp{}
	}

	// By default, we're in CloudRun
	return &CloudRun{}
}
