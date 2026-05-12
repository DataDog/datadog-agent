// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logssourceimpl implements the logssource component.
package logssourceimpl

import (
	logssource "github.com/DataDog/datadog-agent/comp/anomalydetection/logssource/def"
)

type component struct{}

// NewComponent creates a no-op logssource component.
func NewComponent() logssource.Component {
	return component{}
}
