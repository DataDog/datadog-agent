// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package providers

import "github.com/DataDog/datadog-agent/pkg/collector/check"

// ConfigProvider is the interface that wraps the Collect method
//
// Collect is responsible of populating a list of CheckConfig instances
// by retrieving configuration patterns from external resources: files
// on disk, databases, environment variables are just few examples.
//
// Any type implementing the interface will take care of any dependency
// or data needed to access the resource providing the configuration.
type ConfigProvider interface {
	Collect() ([]check.Config, error)
}
