// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package exprfilter package provides utilities for compiling and running expressions
package exprfilter

import (
	"github.com/expr-lang/expr"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DummyCall is a function that compiles and runs a simple expression
func DummyCall() {
	prg, _ := expr.Compile("true || false")
	o, _ := expr.Run(prg, nil)
	if o != nil {
		log.Infof("Expression evaluation result: %v", o)
	} else {
		log.Warnf("Expression evaluation returned nil, this is unexpected")
	}
}
