// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package check

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

// Loader is the interface wrapping the operations to load a check from
// different sources, like Python modules or Go objects.
//
// A single check is loaded for the given `instance` YAML.
type Loader interface {
	Name() string
	Load(config integration.Config, instance integration.Data) (Check, error)
}
