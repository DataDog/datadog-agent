// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package metadata

import "github.com/DataDog/datadog-agent/pkg/serializer"

// Collector is anything capable to collect and send metadata payloads
// through the forwarder.
// A Metadata Collector normally uses a Metadata Provider to fill the payload.
type Collector interface {
	Send(s *serializer.Serializer) error
}
