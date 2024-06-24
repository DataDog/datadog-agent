// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("pkgconfig", New)
}

// type MySettings struct {
// 	One   string    `json:"one"`
// 	Two   []Element `json:"two"`
// 	Three Element   `json:"three"`
// }

// type Element struct {
// 	Name string `json:"name"`
// }

type PluginExample struct {
}

func New(settings any) (register.LinterPlugin, error) {
	// The configuration type will be map[string]any or []interface, it depends on your configuration.
	// You can use https://github.com/go-viper/mapstructure to convert map to struct.

	// s, err := register.DecodeSettings[MySettings](settings)
	// if err != nil {
	// 	return nil, err
	// }

	return &PluginExample{}, nil
}

func (f *PluginExample) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{
		{
			Name: "pkgconfig",
			Doc:  "ensure pkg/config is not used inside comp folder",
			Run:  f.run,
		},
	}, nil
}

func (f *PluginExample) GetLoadMode() string {
	return register.LoadModeSyntax
}

func (f *PluginExample) run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		if file.Imports == nil {
			continue
		}

		if strings.Contains(pass.Pkg.Path(), "github.com/DataDog/datadog-agent/comp") {
			for _, imp := range file.Imports {
				if strings.Contains(imp.Path.Value, "pkg/config") {
					pass.Report(analysis.Diagnostic{
						Pos:      imp.Pos(),
						End:      imp.End(),
						Category: "components",
						Message:  "pkg/config should not be used inside comp folder",
						SuggestedFixes: []analysis.SuggestedFix{
							{
								Message: "Use the config component instead. Normally you can declare the confg component as part of your component dependencies.",
							},
						},
					})
				}
			}
		}
	}

	return nil, nil
}
