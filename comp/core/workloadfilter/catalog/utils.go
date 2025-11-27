// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cel

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	"os"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/celprogram"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

func createCELExcludeProgram(name string, rules string, objectType workloadfilter.ResourceType, logger log.Component) program.FilterProgram {
	excludeProgram, excludeErr := celprogram.CreateCELProgram(rules, objectType)
	if excludeErr != nil {
		logger.Criticalf(`failed to compile '%s' from 'cel_workload_exclude' filters: %v`, name, excludeErr)
		logger.Flush()
		os.Exit(1)
	}
	return program.CELProgram{
		Name:                 name,
		Exclude:              excludeProgram,
		InitializationErrors: nil,
	}
}
