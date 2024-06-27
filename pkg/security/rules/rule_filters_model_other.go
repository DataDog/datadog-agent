// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Package rules holds rules related files
package rules

import (
	"os"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// RuleFilterEvent represents a rule filtering event
type RuleFilterEvent struct {
	origin string
}

// RuleFilterModel represents a rule fitlering model
type RuleFilterModel struct {
	origin string
}

// NewRuleFilterModel returns a new rule filtering model
func NewRuleFilterModel(origin string) (*RuleFilterModel, error) {
	return &RuleFilterModel{
		origin: origin,
	}, nil
}

// NewEvent returns a new rule filtering event
func (m *RuleFilterModel) NewEvent() eval.Event {
	return &RuleFilterEvent{
		origin: m.origin,
	}
}

// GetEvaluator returns a new evaluator for a rule filtering field
func (m *RuleFilterModel) GetEvaluator(field eval.Field, _ eval.RegisterID) (eval.Evaluator, error) {
	switch field {
	case "kernel.version.major", "kernel.version.minor", "kernel.version.patch",
		"kernel.version.abi", "kernel.version.flavor":
		return &eval.IntEvaluator{
			Value: 0,
			Field: field,
		}, nil

	case "os":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string { return runtime.GOOS },
			Field:   field,
		}, nil
	case "os.id", "os.platform_id", "os.version_id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string { return runtime.GOOS },
			Field:   field,
		}, nil

	case "os.is_amazon_linux", "os.is_cos", "os.is_debian", "os.is_oracle", "os.is_rhel", "os.is_rhel7",
		"os.is_rhel8", "os.is_sles", "os.is_sles12", "os.is_sles15":
		return &eval.BoolEvaluator{
			Value: false,
			Field: field,
		}, nil

	case "envs":
		return &eval.StringArrayEvaluator{
			Values: os.Environ(),
		}, nil
	case "origin":
		return &eval.StringEvaluator{
			Value: m.origin,
			Field: field,
		}, nil
	case "hostname":
		return &eval.StringEvaluator{
			Value: getHostname(),
			Field: field,
		}, nil
	}

	return nil, &eval.ErrFieldNotFound{Field: field}
}

// GetFieldValue returns the value of the given field
func (e *RuleFilterEvent) GetFieldValue(field eval.Field) (interface{}, error) {
	switch field {
	case "kernel.version.major", "kernel.version.minor", "kernel.version.patch",
		"kernel.version.abi", "kernel.version.flavor":
		return 0, nil

	case "os":
		return runtime.GOOS, nil
	case "os.id", "os.platform_id", "os.version_id":
		return runtime.GOOS, nil

	case "os.is_amazon_linux", "os.is_cos", "os.is_debian", "os.is_oracle", "os.is_rhel", "os.is_rhel7",
		"os.is_rhel8", "os.is_sles", "os.is_sles12", "os.is_sles15":
		return false, nil

	case "envs":
		return os.Environ(), nil
	case "origin":
		return e.origin, nil
	case "hostname":
		return getHostname(), nil
	}

	return nil, &eval.ErrFieldNotFound{Field: field}
}
