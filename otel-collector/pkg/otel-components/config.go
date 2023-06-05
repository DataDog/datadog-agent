package otelcomponents

import "github.com/DataDog/datadog-agent/comp/core/config"

type OtelConfig[cfg any] struct {
	config   cfg
	fieldMap map[string]any
}

var _ config.Component = &OtelConfig[string]{}
