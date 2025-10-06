// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package create provides the constructor for the config
package create

import (
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/nodetreemodel"
	"github.com/DataDog/datadog-agent/pkg/config/teeconfig"
	"github.com/DataDog/datadog-agent/pkg/config/viperconfig"
)

// NewConfig returns a config with the given name. Implementation of the
// config is chosen by an env var
func NewConfig(name string) model.BuildableConfig {
	// Configure Datadog global configuration
	envvar := os.Getenv("DD_CONF_NODETREEMODEL")
	// Possible values for DD_CONF_NODETREEMODEL:
	// - "enable":    Use the nodetreemodel for the config, instead of viper
	// - "tee":       Construct both viper and nodetreemodel. Write to both, only read from viper
	// - "unmarshal": Use viper for the config but the reflection based version of UnmarshalKey which used some of
	//                nodetreemodel internals
	// - other:       Use viper for the config
	if envvar == "enable" {
		return nodetreemodel.NewNodeTreeConfig(name, "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
	} else if envvar == "tee" {
		viperImpl := viperconfig.NewViperConfig(name, "DD", strings.NewReplacer(".", "_"))         // nolint: forbidigo // legit use case
		nodetreeImpl := nodetreemodel.NewNodeTreeConfig(name, "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
		return teeconfig.NewTeeConfig(viperImpl, nodetreeImpl)
	}
	return viperconfig.NewViperConfig(name, "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
}
