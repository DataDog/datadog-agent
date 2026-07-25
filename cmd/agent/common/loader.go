// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	confad "github.com/DataDog/datadog-agent/pkg/config/autodiscovery"
)

// LoadComponents configures several common Agent components:
// tagger, collector, scheduler and autodiscovery
func LoadComponents(ac autodiscovery.Component, config config.Component) {
	confad.LoadComponents(ac, config)
}
