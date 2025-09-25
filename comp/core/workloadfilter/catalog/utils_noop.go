// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !cel

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
)

type noopFilterProgram struct{}

func (n noopFilterProgram) Evaluate(entity workloadfilter.Filterable) (workloadfilter.Result, []error) {
	return workloadfilter.Unknown, nil
}

func (n noopFilterProgram) GetInitializationErrors() []error {
	return nil
}

func createFromOldFilters(_ string, _, _ []string, _ workloadfilter.ResourceType, logger log.Component) program.FilterProgram {
	return noopFilterProgram{}
}
