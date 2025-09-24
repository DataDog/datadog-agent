// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fxutiltest provides a linter for ensuring each fxutil.OneShot function has a corresponding unit test
package fxutiltest

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("fxutiltest", New)
}

type fxutilTestPlugin struct {
}

// New returns a new fxutil test linter plugin
func New(any) (register.LinterPlugin, error) {
	return &fxutilTestPlugin{}, nil
}

// BuildAnalyzers returns the analyzers for the plugin
func (f *fxutilTestPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{
		{
			Name: "fxutiltest",
			Doc:  "ensure each fxutil.OneShot function has a corresponding unit test",
			Run:  f.run,
		},
	}, nil
}

// GetLoadMode returns the load mode for the plugin
func (f *fxutilTestPlugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

func (f *fxutilTestPlugin) run(pass *analysis.Pass) (interface{}, error) {
	var sourceFiles []*ast.File
	var testFiles []*ast.File

	// Separate source files and test files
	for _, f := range pass.Files {
		fname := pass.Fset.File(f.Pos()).Name()

		if strings.HasSuffix(fname, "_test.go") {
			fmt.Printf("DEBUG: Adding test file: %s\n", fname)
			testFiles = append(testFiles, f)
		} else {
			fmt.Printf("DEBUG: Adding source file: %s\n", fname)
			sourceFiles = append(sourceFiles, f)
		}
	}
	fmt.Printf("DEBUG: Total files - Source: %d, Test: %d\n", len(sourceFiles), len(testFiles))
	// Extract cobra commands from source files
	cobraCommands := extractCobraCommands(sourceFiles, pass)
	if len(cobraCommands) == 0 {
		return nil, nil
	}

	// If no test files are available, we can't determine what's tested
	if len(testFiles) == 0 {
		fmt.Printf("DEBUG: No test files available, skipping analysis\n")
		return nil, nil
	}

	// Extract tested commands from test files
	testedCommands := extractTestedCommands(testFiles, pass.TypesInfo)

	fmt.Printf("DEBUG: Found %d tested commands\n", len(testedCommands))
	for i, test := range testedCommands {
		fmt.Printf("DEBUG: Test %d: %v\n", i, test.CommandLine)
	}

	// Compare and report missing tests
	for _, cmd := range cobraCommands {
		if !isCommandTested(cmd, testedCommands) {
			pass.Report(analysis.Diagnostic{
				Pos:      cmd.Pos,
				End:      cmd.End,
				Category: "fxutiltest",
				Message:  fmt.Sprintf("Cobra command '%s' uses fxutil.OneShot but has no corresponding test", cmd.Use),
			})
		}
	}

	return nil, nil
}

type CobraCommand struct {
	Use      string
	FullPath []string // e.g., ["dogstatsd", "top"]
	Pos      token.Pos
	End      token.Pos
}

type TestedCommand struct {
	CommandLine []string // e.g., ["dogstatsd", "top"]
	Function    string   // function being tested
}

// extractCobraCommands finds all cobra commands that use fxutil.OneShot
func extractCobraCommands(files []*ast.File, pass *analysis.Pass) []CobraCommand {
	var commands []CobraCommand

	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			cl, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}

			// Resolve the type
			typ := pass.TypesInfo.TypeOf(cl)
			if typ == nil {
				// Check if this looks like a cobra command by structure
				if hasCobraCommandStructure(cl) {
					cmd := extractCommandInfo(cl, pass.TypesInfo)
					if cmd != nil && cmd.Use != "" && hasOneShotInRunE(cl, pass.TypesInfo) {
						commands = append(commands, *cmd)
					}
				}
				return true
			}

			// Check if type is "invalid type" (common when imports can't be resolved in tests)
			if typ.String() == "invalid type" {
				// Check if this looks like a cobra command by structure
				if hasCobraCommandStructure(cl) {
					cmd := extractCommandInfo(cl, pass.TypesInfo)
					if cmd != nil && cmd.Use != "" && hasOneShotInRunE(cl, pass.TypesInfo) {
						commands = append(commands, *cmd)
					}
				}
				return true
			}

			// Check if it's a proper cobra.Command type
			if isCobraCommandType(typ) {
				cmd := extractCommandInfo(cl, pass.TypesInfo)
				if cmd != nil && cmd.Use != "" && hasOneShotInRunE(cl, pass.TypesInfo) {
					commands = append(commands, *cmd)
				}
			}

			return true
		})
	}

	return commands
}

// extractTestedCommands finds all commands being tested with fxutil test functions
func extractTestedCommands(files []*ast.File, info *types.Info) []TestedCommand {
	var tested []TestedCommand

	for _, file := range files {
		fmt.Printf("DEBUG: Checking test file for fxutil test calls\n")
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			fmt.Printf("DEBUG: Found function call\n")

			// Check if this is a call to fxutil test function
			if !isFxutilTestCall(call, info) {
				// Also check structurally when type info is not available
				if !isFxutilTestCallStructural(call) {
					return true
				}
				fmt.Printf("DEBUG: Found fxutil test call (structural)\n")
			} else {
				fmt.Printf("DEBUG: Found fxutil test call (typed)\n")
			}

			// Extract command line and function from the test call
			testCmd := extractTestCommandInfo(call)
			if testCmd != nil {
				fmt.Printf("DEBUG: Extracted test command: %v\n", testCmd.CommandLine)
				tested = append(tested, *testCmd)
			} else {
				fmt.Printf("DEBUG: Could not extract test command info\n")
			}

			return true
		})
	}

	return tested
}

// hasCobraCommandStructure checks if a composite literal has the structure of a cobra.Command
func hasCobraCommandStructure(cl *ast.CompositeLit) bool {
	hasUse := false
	hasRunE := false

	for _, elt := range cl.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if ident, ok := kv.Key.(*ast.Ident); ok {
				switch ident.Name {
				case "Use":
					if _, ok := kv.Value.(*ast.BasicLit); ok {
						hasUse = true
					}
				case "RunE":
					hasRunE = true
				}
			}
		}
	}

	return hasUse && hasRunE
}

// isCobraCommandType checks if a type is cobra.Command or *cobra.Command
func isCobraCommandType(typ types.Type) bool {
	// Handle pointer types
	if ptr, ok := typ.(*types.Pointer); ok {
		typ = ptr.Elem()
	}
	// Check if it's a named type from cobra package
	if named, ok := typ.(*types.Named); ok {
		obj := named.Obj()
		if obj.Name() == "Command" && obj.Pkg() != nil {
			return obj.Pkg().Path() == "github.com/spf13/cobra"
		}
	}
	return false
}

// isCobraCommand checks if a composite literal is a cobra.Command
func isCobraCommand(compLit *ast.CompositeLit, info *types.Info) bool {
	// First check if there's explicit type information
	if compLit.Type != nil {
		if sel, ok := compLit.Type.(*ast.SelectorExpr); ok {
			if sel.Sel.Name == "Command" {
				if ident, ok := sel.X.(*ast.Ident); ok {
					if obj := info.Uses[ident]; obj != nil {
						if pkg := obj.Pkg(); pkg != nil {
							return pkg.Path() == "github.com/spf13/cobra"
						}
					}
				}
			}
		}
	}

	// Check type info directly - this works for composite literals in slices
	if typ := info.TypeOf(compLit); typ != nil {
		// Handle pointer types
		if ptr, ok := typ.(*types.Pointer); ok {
			typ = ptr.Elem()
		}
		// Check if it's a named type from cobra package
		if named, ok := typ.(*types.Named); ok {
			obj := named.Obj()
			if obj.Name() == "Command" && obj.Pkg() != nil {
				return obj.Pkg().Path() == "github.com/spf13/cobra"
			}
		}
	}

	return false
}

// extractCommandInfo extracts Use field and position from cobra.Command
func extractCommandInfo(compLit *ast.CompositeLit, _ *types.Info) *CobraCommand {
	cmd := &CobraCommand{
		Pos: compLit.Pos(),
		End: compLit.End(),
	}

	for _, elt := range compLit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if ident, ok := kv.Key.(*ast.Ident); ok && ident.Name == "Use" {
				if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					// Remove quotes from string literal
					cmd.Use = strings.Trim(lit.Value, "\"")
				}
			}
		}
	}

	return cmd
}

// hasOneShotInRunE checks if the RunE field contains a call to fxutil.OneShot
func hasOneShotInRunE(compLit *ast.CompositeLit, info *types.Info) bool {
	for _, elt := range compLit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if ident, ok := kv.Key.(*ast.Ident); ok && ident.Name == "RunE" {
				// Check if the RunE function contains fxutil.OneShot
				return containsOneShotCall(kv.Value, info) || containsOneShotCallStructural(kv.Value)
			}
		}
	}
	return false
}

// containsOneShotCall recursively checks if a node contains fxutil.OneShot call
func containsOneShotCall(node ast.Node, info *types.Info) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if isOneShotCall(call, info) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// containsOneShotCallStructural checks structurally for fxutil.OneShot calls when type info is unavailable
func containsOneShotCallStructural(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				// Check if it looks like fxutil.OneShot
				if sel.Sel.Name == "OneShot" {
					if ident, ok := sel.X.(*ast.Ident); ok {
						if ident.Name == "fxutil" {
							found = true
							return false
						}
					}
				}
			}
		}
		return true
	})
	return found
}

// isFxutilTestCall checks if a call is to fxutil test function
func isFxutilTestCall(call *ast.CallExpr, info *types.Info) bool {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		testFunctions := map[string]bool{
			"TestOneShotSubcommand": true,
			"TestOneShot":           true,
			"TestRun":               true,
		}

		if testFunctions[sel.Sel.Name] {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if obj := info.Uses[ident]; obj != nil {
					if pkg := obj.Pkg(); pkg != nil {
						return pkg.Path() == "github.com/DataDog/datadog-agent/pkg/util/fxutil"
					}
				}
			}
		}
	}
	return false
}

// isFxutilTestCallStructural checks structurally for fxutil test function calls
func isFxutilTestCallStructural(call *ast.CallExpr) bool {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		testFunctions := map[string]bool{
			"TestOneShotSubcommand": true,
			"TestOneShot":           true,
			"TestRun":               true,
		}

		if testFunctions[sel.Sel.Name] {
			if ident, ok := sel.X.(*ast.Ident); ok {
				return ident.Name == "fxutil"
			}
		}
	}
	return false
}

// extractTestCommandInfo extracts command line from fxutil test calls
func extractTestCommandInfo(call *ast.CallExpr) *TestedCommand {
	if len(call.Args) < 3 {
		return nil
	}

	// The command line is typically the 3rd argument (index 2)
	cmdArg := call.Args[2]

	// Extract string slice from []string{"cmd", "subcommand"}
	if compLit, ok := cmdArg.(*ast.CompositeLit); ok {
		var commandLine []string
		for _, elt := range compLit.Elts {
			if lit, ok := elt.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				commandLine = append(commandLine, strings.Trim(lit.Value, "\""))
			}
		}

		if len(commandLine) > 0 {
			return &TestedCommand{
				CommandLine: commandLine,
				Function:    "unknown", // Could extract this from 4th arg if needed
			}
		}
	}

	return nil
}

// isCommandTested checks if a cobra command has corresponding tests
func isCommandTested(cmd CobraCommand, tested []TestedCommand) bool {
	fmt.Printf("DEBUG: Checking if command '%s' is tested\n", cmd.Use)
	for i, test := range tested {
		fmt.Printf("DEBUG: Test %d: commandLine=%v, lastElement='%s'\n", i, test.CommandLine,
			func() string {
				if len(test.CommandLine) > 0 {
					return test.CommandLine[len(test.CommandLine)-1]
				}
				return "empty"
			}())
		if len(test.CommandLine) > 0 && test.CommandLine[len(test.CommandLine)-1] == cmd.Use {
			fmt.Printf("DEBUG: Command '%s' IS tested\n", cmd.Use)
			return true
		}
	}
	fmt.Printf("DEBUG: Command '%s' is NOT tested\n", cmd.Use)
	return false
}

// formatCommandLine formats command path for error messages
func formatCommandLine(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return fmt.Sprintf("[%s]", strings.Join(path, ", "))
}

// isOneShotCall checks if a call expression is a call to fxutil.OneShot
func isOneShotCall(call *ast.CallExpr, info *types.Info) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	// Check if the selector is "OneShot"
	if sel.Sel.Name != "OneShot" {
		return false
	}

	// Check if it's from the fxutil package
	if ident, ok := sel.X.(*ast.Ident); ok {
		if obj := info.Uses[ident]; obj != nil {
			if pkg := obj.Pkg(); pkg != nil {
				return pkg.Path() == "github.com/DataDog/datadog-agent/pkg/util/fxutil"
			}
		}
	}

	return false
}

// shouldCheckFile determines if we should check this file for OneShot usage
func shouldCheckFile(filename string) bool {
	// Only check files in cmd, pkg/cli, and comp directories
	normalizedPath := filepath.ToSlash(filename)
	return strings.Contains(normalizedPath, "/cmd/") ||
		strings.Contains(normalizedPath, "/pkg/cli/") ||
		strings.Contains(normalizedPath, "/comp/")
}
