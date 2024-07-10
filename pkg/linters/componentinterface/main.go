// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package componentinterface

import (
	"go/ast"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("componentinterface", New)
}

type componentInterfacePlugin struct {
}

func New(_ any) (register.LinterPlugin, error) {
	return &componentInterfacePlugin{}, nil
}

func (f *componentInterfacePlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{
		{
			Name: "componentinterface",
			Doc:  "ensure Component interface do not import third party packages",
			Run:  f.run,
		},
	}, nil
}

func (f *componentInterfacePlugin) GetLoadMode() string {
	return register.LoadModeSyntax
}

func (f *componentInterfacePlugin) run(pass *analysis.Pass) (interface{}, error) {
	for _, f := range pass.Files {
		var componentInterface bool

		ast.Inspect(f, func(node ast.Node) bool {
			switch t := node.(type) {
			// find variable declarations
			case *ast.TypeSpec:
				// which are public
				if t.Name.IsExported() {
					switch t.Type.(type) {
					// and are interfaces
					case *ast.InterfaceType:
						if t.Name.Name == "Component" {
							componentInterface = true
							return false
						}
					}
				}
			}
			return true
		})

		if componentInterface {
			for _, imp := range f.Imports {
				if strings.Contains(imp.Path.Value, ".") && strings.Contains(imp.Path.Value, "/") {
					// This chekc is incomplete, we would need to add more checks to ensure that the import is from a third party package
					if !strings.Contains(imp.Path.Value, "github.com/datadog-agent/") {
						pass.Report(analysis.Diagnostic{
							Pos:      imp.Pos(),
							End:      imp.End(),
							Category: "components",
							Message:  "Component interface import third party package. Importing third party package as part of the component interface is not recommend as it makes exporting the interface more difficult.",
							SuggestedFixes: []analysis.SuggestedFix{
								{
									Message: "Instead of using third party package values, consider using a local interface or a concreate type, such string, int, etc.",
								},
							},
						})
					}
				}
			}
		}
	}

	return nil, nil
}
