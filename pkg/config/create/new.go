// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package create provides the constructor for the config
package create

import (
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/buildschema"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/nodetreemodel"
)

// NewConfig returns a config with the given name.
func NewConfig(name string) model.BuildableConfig {
	if len(os.Args) >= 2 && os.Args[1] == "createschema" {
		return buildschema.NewSchemaBuilder(name, "DD", strings.NewReplacer(".", "_"))
	}

	return nodetreemodel.NewNodeTreeConfig(name, "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
}
