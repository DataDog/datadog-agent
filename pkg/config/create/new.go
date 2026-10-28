// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package create provides the constructor for the config
package create

import (
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/nodetreemodel"
)

// NewConfig returns a config with the given name.
func NewConfig(name string) model.BuildableConfig {
	schemaID := model.CoreSchema
	if name == "system-probe" {
		schemaID = model.SystemProbeSchema
	}
	return nodetreemodel.NewNodeTreeConfig(name, schemaID) // nolint: forbidigo // legit use case
}
