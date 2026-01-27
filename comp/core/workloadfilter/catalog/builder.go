// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	"regexp"
	"strings"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// ProgramBuilder provides methods for creating filter programs with shared dependencies
type ProgramBuilder struct {
	config         *FilterConfig
	logger         log.Component
	telemetryStore *telemetry.Store
}

// NewProgramBuilder creates a new ProgramBuilder with the given dependencies
func NewProgramBuilder(config *FilterConfig, logger log.Component, telemetryStore *telemetry.Store) *ProgramBuilder {
	return &ProgramBuilder{
		config:         config,
		logger:         logger,
		telemetryStore: telemetryStore,
	}
}

// CreateCELProgram creates a CEL-based filter program (uses build-tagged createCELProgram helper)
func (b *ProgramBuilder) CreateCELProgram(id workloadfilter.FilterIdentifier, product workloadfilter.Product) program.FilterProgram {
	name := id.GetFilterName()
	resourceType := id.TargetResource()
	rule := b.config.GetCELRulesForProduct(product, resourceType)
	return createCELProgram(name, rule, resourceType, b.telemetryStore, b.logger)
}

// CreateLegacyProgram creates a legacy container filter program
func (b *ProgramBuilder) CreateLegacyProgram(id workloadfilter.FilterIdentifier, include, exclude []string) program.FilterProgram {
	name := id.GetFilterName()

	var initErrors []error
	filter, err := containers.NewFilter(containers.GlobalFilter, include, exclude)
	if err != nil {
		initErrors = append(initErrors, err)
		b.logger.Warnf("Failed to create filter '%s': %v", name, err)
	}

	return program.LegacyFilterProgram{
		Name:                 name,
		Filter:               filter,
		InitializationErrors: initErrors,
	}
}

// CreateAnnotationsProgram creates an annotations-based filter program
func (b *ProgramBuilder) CreateAnnotationsProgram(id workloadfilter.FilterIdentifier, excludePrefix string) program.FilterProgram {
	return program.AnnotationsProgram{
		Name:          id.GetFilterName(),
		ExcludePrefix: excludePrefix,
	}
}

// CreateRegexProgram creates a regex-based filter program
func (b *ProgramBuilder) CreateRegexProgram(
	id workloadfilter.FilterIdentifier,
	patterns []string,
	extractFieldFunc func(entity workloadfilter.Filterable) string,
) program.FilterProgram {
	name := id.GetFilterName()
	var initErrors []error

	if len(patterns) == 0 {
		return &program.RegexProgram{
			Name:                 name,
			ExtractField:         extractFieldFunc,
			InitializationErrors: initErrors,
		}
	}

	combinedPattern := strings.Join(patterns, "|")
	excludeRegex, err := regexp.Compile(combinedPattern)
	if err != nil {
		initErrors = append(initErrors, err)
		b.logger.Warnf("Error compiling regex pattern for %s: %v", name, err)
		return &program.RegexProgram{
			Name:                 name,
			ExtractField:         extractFieldFunc,
			InitializationErrors: initErrors,
		}
	}

	return &program.RegexProgram{
		Name:                 name,
		ExcludeRegex:         []*regexp.Regexp{excludeRegex},
		ExtractField:         extractFieldFunc,
		InitializationErrors: initErrors,
	}
}
