// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package selector

import (
	compression "github.com/DataDog/datadog-agent/comp/serializer/compression/def"
	implnoop "github.com/DataDog/datadog-agent/comp/serializer/compression/impl-noop"
)

type CompressorFactory struct{}

// NewCompressorFactory creates a new compression factory.
func NewCompressorFactory() compression.Factory {
	return &CompressorFactory{}
}

// NewNoopCompressor returns an identity compression component that does no compression.
func (*CompressorFactory) NewNoopCompressor() compression.Component {
	return implnoop.NewComponent().Comp
}
