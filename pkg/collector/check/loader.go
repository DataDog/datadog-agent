// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

// Loader is the interface wrapping the operations to load a check from
// different sources, like Python modules or Go objects.
//
// A single check is loaded for the given `instance` YAML.
type Loader interface {
	Name() string
	Load(senderManager sender.SenderManager, config integration.Config, instance integration.Data, instanceIndex int) (Check, error)
}

// LoaderSupport describes whether a loader can claim a config instance without
// constructing or configuring a check.
type LoaderSupport int

const (
	// LoaderSupportUnknown means the loader cannot answer without the normal
	// Load path. Callers that need side-effect-free resolution should treat this
	// as ambiguous when it appears before a supported loader.
	LoaderSupportUnknown LoaderSupport = iota
	// LoaderSupportUnsupported means the loader does not claim the instance.
	LoaderSupportUnsupported
	// LoaderSupportSupported means the loader claims the instance.
	LoaderSupportSupported
)

// MetadataLoader can resolve loader support without constructing a check.
type MetadataLoader interface {
	SupportsConfig(integration.Config, integration.Data) LoaderSupport
}
