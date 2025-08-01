// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils is a compliance internal submodule implementing various utilies.
package utils

import (
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// NewContainerFilter returns a new include/exclude filter for containers from runtime security
func NewContainerFilter() (*containers.Filter, error) {
	return common.NewContainerFilter(pkgconfigsetup.SystemProbe(), "runtime_security_config.")
}
