// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
package compliance

import (
	"errors"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// ErrResourceNotSupported is returned when resource type is not supported by CheckBuilder
var ErrResourceNotSupported = errors.New("resource type not supported")

// CheckBuilder defines an interface to build checks from rules
type CheckBuilder interface {
	CheckFromRule(meta *SuiteMeta, rule *Rule) (check.Check, error)
}

// NewCheckBuilder constructs a check builder
func NewCheckBuilder(checkInterval time.Duration, reporter Reporter) CheckBuilder {
	return &checkBuilder{
		checkInterval: checkInterval,
		reporter:      reporter,
	}
}

type checkBuilder struct {
	checkInterval time.Duration
	reporter      Reporter
}

func (b *checkBuilder) CheckFromRule(meta *SuiteMeta, rule *Rule) (check.Check, error) {
	// TODO: evaluate the rule scope here and return an error for rules
	// which are not applicable
	for _, resource := range rule.Resources {

		// TODO: there has to be some logic here to allow for composite checks,
		// to support overrides of reported values, e.g.:
		// default value checked in a file but can be overwritten by a process
		// argument.

		if resource.File != nil {
			return b.fileCheck(meta, rule.ID, resource.File)
		} /* else if {
			// ... other supported resources
		} */
	}
	return nil, ErrResourceNotSupported
}

func (b *checkBuilder) fileCheck(meta *SuiteMeta, ruleID string, file *File) (check.Check, error) {
	// TODO: validate config for the file here
	return &fileCheck{
		baseCheck: b.baseCheck(ruleID, meta),
		File:      file,
	}, nil
}

func (b *checkBuilder) baseCheck(ruleID string, meta *SuiteMeta) baseCheck {
	return baseCheck{
		id:        check.ID(ruleID),
		interval:  b.checkInterval,
		reporter:  b.reporter,
		framework: meta.Framework,
		version:   meta.Version,

		ruleID: ruleID,
	}
}
