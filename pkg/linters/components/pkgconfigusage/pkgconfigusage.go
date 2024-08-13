// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pkgconfigusage provides a linter for ensuring pkg/config is not used inside comp folder
package pkgconfigusage

import (
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

// We can replace during tests
var componentPath = "github.com/DataDog/datadog-agent/comp"

func init() {
	register.Plugin("pkgconfigusage", New)
}

type pkgconfigUsagePlugin struct {
}

// New returns a new config linter plugin
func New(any) (register.LinterPlugin, error) {
	return &pkgconfigUsagePlugin{}, nil
}

// BuildAnalyzers returns the analyzers for the plugin
func (f *pkgconfigUsagePlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{
		{
			Name: "pkgconfigusage",
			Doc:  "ensure github.com/DataDog/datadog-agent/pkg/config is not used inside comp folder",
			Run:  f.run,
		},
	}, nil
}

// GetLoadMode returns the load mode for the plugin
func (f *pkgconfigUsagePlugin) GetLoadMode() string {
	return register.LoadModeSyntax
}

func (f *pkgconfigUsagePlugin) run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {

		if !strings.HasPrefix(pass.Pkg.Path(), componentPath) {
			continue
		}

		for _, imp := range file.Imports {
			if imp.Path.Value == `"github.com/DataDog/datadog-agent/pkg/config"` {
				pass.Report(analysis.Diagnostic{
					Pos:      imp.Pos(),
					End:      imp.End(),
					Category: "components",
					Message:  "github.com/DataDog/datadog-agent/pkg/config should not be used inside comp folder",
					SuggestedFixes: []analysis.SuggestedFix{
						{
							Message: "Use the config component instead, by declaring it as part of your component's dependencies.",
						},
					},
				})
			}
		}
	}

	return nil, nil
}
