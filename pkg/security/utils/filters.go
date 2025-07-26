// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils is a compliance internal submodule implementing various utilies.
package utils

import (
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// NewContainerFilter returns a new include/exclude filter for containers
func NewContainerFilter() (*containers.Filter, error) {
	includeList := pkgconfigsetup.Datadog().GetStringSlice("runtime_security_config.container_include")
	excludeList := pkgconfigsetup.Datadog().GetStringSlice("runtime_security_config.container_exclude")

	if pkgconfigsetup.Datadog().GetBool("runtime_security_config.exclude_pause_container") {
		excludeList = append(excludeList, containers.GetPauseContainerExcludeList()...)
	}

	return containers.NewFilter(containers.GlobalFilter, includeList, excludeList)
}
