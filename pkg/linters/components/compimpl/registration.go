// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compimpl

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

// RegisterLinter register a new linter.
func RegisterLinter(name string, doc string, run func(*analysis.Pass) (interface{}, error)) {
	register.Plugin(name, func(any) (register.LinterPlugin, error) {
		return &linterImpl{name: name, doc: doc, run: run}, nil
	})
}

type linterImpl struct {
	name string
	doc  string
	run  func(*analysis.Pass) (interface{}, error)
}

// BuildAnalyzers returns the analyzers for the plugin
func (f *linterImpl) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{
		{
			Name: f.name,
			Doc:  f.doc,
			Run:  f.run,
		},
	}, nil
}

// GetLoadMode returns the load mode for the plugin
func (f *linterImpl) GetLoadMode() string {
	return register.LoadModeSyntax
}
