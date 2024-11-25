// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package selector

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/def"
)

// Requires contains the config for Compression
type Requires struct {
	Cfg config.Component
}

// Provides contains the compression component
type Provides struct {
	Comp compression.Component
}
