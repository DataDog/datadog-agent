// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubelet && docker

// Package tailerfactory implements the logic required to determine which kind
// of tailer to use for a container-related LogSource, and to create that tailer.
package tailerfactory

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func (tf *factory) makeAPITailer(source *sources.LogSource) (Tailer, error) {
	return nil, errors.New("API tailing is unavailable")
}
