// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common holds common related files
package common

import (
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// NewContainerFilter returns a new include/exclude filter for containers
func NewContainerFilter(cfg model.Config, prefix string) (*containers.Filter, error) {
	includeList := cfg.GetStringSlice(prefix + "container_include")
	excludeList := cfg.GetStringSlice(prefix + "container_exclude")

	if cfg.GetBool(prefix + "exclude_pause_container") {
		excludeList = append(excludeList, containers.GetPauseContainerExcludeList()...)
	}

	return containers.NewFilter(containers.GlobalFilter, includeList, excludeList)
}
