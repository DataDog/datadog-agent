// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !otlp

// Package collector contains a no-op implementation of the collector
package collector

import (
	collector "github.com/DataDog/datadog-agent/comp/otelcol/collector/def"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/datatype"
)

type noopImpl struct{}

func (i *noopImpl) Status() datatype.CollectorStatus {
	return datatype.CollectorStatus{}
}

// NewComponent returns a new instance of the collector component.
func NewComponent() collector.Component {
	return &noopImpl{}
}
