// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package id

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

// ID is the representation of the unique ID of a Check instance
type ID string

// BuildID returns an unique ID for a check name and its configuration
func BuildID(checkName string, integrationConfigDigest uint64, instance, initConfig integration.Data) ID {
	panic("not called")
}

// IDToCheckName returns the check name from a check ID
//
//nolint:revive // TODO(AML) Fix revive linter
func IDToCheckName(id ID) string {
	panic("not called")
}
