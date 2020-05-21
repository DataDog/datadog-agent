// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// Package checks implements Compliance Agent checks
package checks

import (
	"errors"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/compliance"
)

// ErrResourceNotSupported is returned when resource type is not supported by CheckBuilder
var ErrResourceNotSupported = errors.New("resource type not supported")

// Builder defines an interface to build checks from rules
type Builder interface {
	CheckFromRule(meta *compliance.SuiteMeta, rule *compliance.Rule) (check.Check, error)
}

// BuilderEnv defines builder environment used to instantiate different checks
type BuilderEnv struct {
	Reporter     compliance.Reporter
	DockerClient DockerClient
	HostRoot     string
}

// NewBuilder constructs a check builder
func NewBuilder(checkInterval time.Duration, env BuilderEnv) Builder {
	var mapper pathMapper
	if len(env.HostRoot) != 0 {
		mapper = func(path string) string {
			return filepath.Join(env.HostRoot, path)
		}
	}
	return &builder{
		checkInterval: checkInterval,
		pathMapper:    mapper,
		env:           env,
	}
}

type builder struct {
	checkInterval time.Duration
	env           BuilderEnv

	pathMapper pathMapper
}

func (b *builder) CheckFromRule(meta *compliance.SuiteMeta, rule *compliance.Rule) (check.Check, error) {
	// TODO: evaluate the rule scope here and return an error for rules
	// which are not applicable
	for _, resource := range rule.Resources {

		// TODO: there has to be some logic here to allow for composite checks,
		// to support overrides of reported values, e.g.:
		// default value checked in a file but can be overwritten by a process
		// argument.
		switch {
		case resource.File != nil:
			return b.fileCheck(meta, rule.ID, resource.File)
		case resource.Docker != nil:
			return b.dockerCheck(meta, rule.ID, resource.Docker)
		case resource.Process != nil:
			return newProcessCheck(b.baseCheck(rule.ID, meta), resource.Process)
		}
	}
	return nil, ErrResourceNotSupported
}

func (b *builder) fileCheck(meta *compliance.SuiteMeta, ruleID string, file *compliance.File) (check.Check, error) {
	// TODO: validate config for the file here

	return &fileCheck{
		baseCheck:  b.baseCheck(ruleID, meta),
		pathMapper: b.pathMapper,
		file:       file,
	}, nil
}

func (b *builder) dockerCheck(meta *compliance.SuiteMeta, ruleID string, dockerResource *compliance.DockerResource) (check.Check, error) {
	// TODO: validate config for the file here
	return &dockerCheck{
		baseCheck:      b.baseCheck(ruleID, meta),
		dockerResource: dockerResource,
		client:         b.env.DockerClient,
	}, nil
}

func (b *builder) baseCheck(ruleID string, meta *compliance.SuiteMeta) baseCheck {
	return baseCheck{
		id:        check.ID(ruleID),
		interval:  b.checkInterval,
		reporter:  b.env.Reporter,
		framework: meta.Framework,
		version:   meta.Version,

		ruleID: ruleID,
	}
}
