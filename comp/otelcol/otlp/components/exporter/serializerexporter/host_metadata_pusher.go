// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package serializerexporter

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata/payload"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

// HostMetadataPusher implements the inframetadata.Push interface
type HostMetadataPusher struct {
	s serializer.MetricSerializer
}

// NewPusher returns a new HostMetadataPusher
func NewPusher(s serializer.MetricSerializer) *HostMetadataPusher {
	return &HostMetadataPusher{s: s}
}

var _ inframetadata.Pusher = (*HostMetadataPusher)(nil)

func (h *HostMetadataPusher) Push(_ context.Context, hm payload.HostMetadata) error {
	fmt.Println("payload.HostMetadata", hm)
	return h.s.SendHostMetadata(&hm)
}
