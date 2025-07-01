// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	filter "github.com/DataDog/datadog-agent/comp/core/filter/def"
	"github.com/DataDog/datadog-agent/comp/core/filter/program"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// ImageContainerPausedProgram creates a program for filtering paused containers
func ImageContainerPausedProgram(config config.Component, logger log.Component) program.CELProgram {
	var excludeList []string
	fmt.Println("Evaluating the boolean: exclude_pause_container", config.GetBool("exclude_pause_container"))
	if config.GetBool("exclude_pause_container") {
		excludeList = containers.GetPauseContainerExcludeList()
	}

	return program.CELProgram{
		Name:    "ImageContainerPausedProgram",
		Exclude: createProgramFromOldFilters(excludeList, filter.ImageType, logger),
	}
}
