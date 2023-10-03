// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkconfig

import (
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

type profileConfigMap map[string]profileConfig

type profileConfig struct {
	DefinitionFile string                              `yaml:"definition_file"`
	Definition     profiledefinition.ProfileDefinition `yaml:"definition"`

	isUserProfile bool `yaml:"-"`
}
