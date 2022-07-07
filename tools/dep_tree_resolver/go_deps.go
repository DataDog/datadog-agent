// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Debug variable can be turned on for development purposes
// to see more output but generally it's not useful for end-use
const Debug = false

// WriteToFile variable selects if we will be writing out final output
// to a file or stdout
const WriteToFile = true

// OutputFileName is our target output filename for the generated
// dependency tree
const OutputFileName = "dependency_tree.txt"

// RootModulePlaceholderVersion denotes what version we will assign to our
// root module as generally we won't have one provided by the Golang tooling
const RootModulePlaceholderVersion = "current"

// Module is a struct that holds a `<VERSION>@<ID>` combination within
// the dependency tree as a unique identifier.
type Module struct {
	Path    string
	Version string
}

// ModuleDep is a structure that holds a bi-directional relationship between
// a parent module and one of its dependencies
type ModuleDep struct {
	Parent Module
	Child  Module
}

// DependencyTree is a structure that identifies a module and all of its
// children in a recursive tree
type DependencyTree struct {
	Mod          *Module
	Dependencies []DependencyTree
}

func runCommand(ctx context.Context, executable string, command ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, executable, command...)
	outputBuf := bytes.NewBuffer(nil)
	cmd.Stderr = outputBuf

	cmdOutput, err := cmd.Output()
	if err != nil {
		fullCommand := strings.Join(append([]string{executable}, command...), " ")
		return nil,
			fmt.Errorf("ERROR! `%s` failed to execute: %s: %v", fullCommand, outputBuf.String(), err)
	}

	return cmdOutput, nil
}

func getRootModule(ctx context.Context) (string, error) {
	goModOutput, err := runCommand(ctx, "go", "mod", "why")
	if err != nil {
		return "", err
	}

	outputLines, err := strings.Split(string(goModOutput), "\n")[1], nil
	if err != nil {
		return "", err
	}

	if len(outputLines) < 2 {
		return "", fmt.Errorf("Could not identify root module using `go mod why` (line count < 2)")
	}

	return outputLines, nil
}

func runGraph(ctx context.Context, rootModule string) ([]ModuleDep, []string, error) {
	goModOutput, err := runCommand(ctx, "go", "mod", "graph")
	if err != nil {
		return nil, nil, err
	}

	outputScannerFunc := func(handlerFunc func(string, string)) error {
		scanner := bufio.NewScanner(bytes.NewBuffer(goModOutput))
		for scanner.Scan() {
			line := scanner.Text()
			fields := strings.Split(line, " ")
			if len(fields) != 2 {
				return fmt.Errorf("error: line didn't have 2 fields: %v", line)
			}
			handlerFunc(fields[0], fields[1])
		}

		return scanner.Err()
	}

	modules := []ModuleDep{}
	moduleNames := map[string]interface{}{}
	err = outputScannerFunc(func(parent string, child string) {
		parentModule := modInfo(parent)
		childModule := modInfo(child)

		if parentModule.Path == rootModule {
			parentModule.Version = RootModulePlaceholderVersion
		}

		dependency := ModuleDep{
			Parent: parentModule,
			Child:  childModule,
		}

		modules = append(modules, dependency)
		moduleNames[parentModule.Path] = nil
		moduleNames[childModule.Path] = nil
	})

	moduleNamesArr := make([]string, 0, len(moduleNames))
	for k := range moduleNames {
		moduleNamesArr = append(moduleNamesArr, k)
	}

	return modules, moduleNamesArr, err
}

func modInfo(modDesc string) Module {
	module := Module{
		Path: modDesc,
	}

	splitIdx := strings.Index(modDesc, "@")
	if splitIdx > 0 {
		module.Path = modDesc[0:splitIdx]
		module.Version = modDesc[splitIdx+1:]
	}

	return module
}

func getActualModuleVersion(ctx context.Context, moduleName string) (Module, error) {
	modWhyOutput, err := runCommand(ctx, "go", "list", "-m", moduleName)
	if err != nil {
		return Module{}, err
	}

	modWhyOutputStr := string(modWhyOutput)

	// If we find the root package, tack on a placeholder version
	if len(strings.Fields(modWhyOutputStr)) == 1 {
		modWhyOutputStr = modWhyOutputStr + " " + RootModulePlaceholderVersion
	}

	// If we have a replacement, we need to figure out what to do with it
	if replaceIdx := strings.Index(modWhyOutputStr, "=>"); replaceIdx >= 0 {
		replacementFields := strings.Fields(modWhyOutputStr)

		if len(replacementFields) == 5 {
			// When a regular replacement is encountered, we will have the id and version in fields
			// 3 and 4 (0-based) respectively.
			modWhyOutputStr = replacementFields[3] + " " + replacementFields[4]
		} else if len(replacementFields) == 4 {
			// If we have a local replacement, we will have the path in field 3 but version in field
			// 2 (0-based).
			modWhyOutputStr = replacementFields[3] + " " + replacementFields[1]
		}
	}

	modWhyFields := strings.Fields(modWhyOutputStr)

	module := Module{
		Path:    modWhyFields[0],
		Version: modWhyFields[1],
	}

	return module, nil
}

func resolveActualVersions(ctx context.Context, moduleNames []string) (map[string]Module, error) {
	modVersionMapping := map[string]Module{}

	for idx, modName := range moduleNames {
		moduleName := modName
		index := idx
		runCtx := ctx

		moduleVersion, err := getActualModuleVersion(runCtx, moduleName)
		if err != nil {
			return nil, err
		}

		if moduleVersion.Path == "" || moduleVersion.Version == "" {
			return nil,
				fmt.Errorf(
					"module %s had an unresolved real version (%v)",
					moduleVersion.Path,
					moduleVersion.Version,
				)
		}

		modVersionMapping[moduleName] = moduleVersion

		fmt.Printf("(%3d/%3d) %s -> %s\n", index+1, len(moduleNames), moduleName, moduleVersion)
	}

	return modVersionMapping, nil
}

func getDependencies(ctx context.Context) (string, []ModuleDep, map[string]Module, error) {
	if Debug {
		fmt.Println("Resolving the root package name...")
	}

	rootModule, err := getRootModule(ctx)
	if err != nil {
		return "", nil, nil, err
	}

	if Debug {
		fmt.Printf("Root module resolved: %s\n", rootModule)
		fmt.Println("Getting the dependency graph...")
	}

	depFlatGraph, moduleNames, err := runGraph(ctx, rootModule)
	if err != nil {
		return "", nil, nil, err
	}

	if Debug {
		for _, module := range depFlatGraph {
			fmt.Printf(
				"%s@%s %s@%s\n",
				module.Parent.Path,
				module.Parent.Version,
				module.Child.Path,
				module.Child.Version,
			)
		}
		fmt.Printf("Resolving the actual version used for modules (%d)...\n", len(moduleNames))
	}

	actualModuleVersions, err := resolveActualVersions(ctx, moduleNames)
	if err != nil {
		return "", nil, nil, err
	}

	if Debug {
		fmt.Printf("Resolved %d modules\n", len(actualModuleVersions))
	}

	return rootModule, depFlatGraph, actualModuleVersions, nil
}

func getModuleDependencies(depFlatGraph []ModuleDep, module Module) []Module {
	dependencies := []Module{}
	for _, moduleDep := range depFlatGraph {
		if moduleDep.Parent == module {
			dependencies = append(dependencies, moduleDep.Child)
		}
	}
	return dependencies
}

func isCircularDependency(module *Module, depPath *[]Module) bool {
	for _, seenModule := range *depPath {
		if (*module).Path == seenModule.Path {
			return true
		}
	}

	return false
}

func resolveRecursive(
	module *Module,
	level int,
	rootDepPath []Module,
	depFlatGraph []ModuleDep,
	actualModuleVersions map[string]Module,
) (*DependencyTree, error) {

	modPath := module.Path
	if Debug {
		fmt.Printf("Resolving dependency (Depth: %d) %s\n", level, modPath)
	}

	actualModule, ok := actualModuleVersions[modPath]
	if !ok {
		return nil,
			fmt.Errorf("ERROR! Could not find actual module mapping for %s (level %d). Path: %+v", modPath, level, rootDepPath)
	}

	modDeps := getModuleDependencies(depFlatGraph, actualModule)

	if Debug {
		fmt.Printf("- Found %d dep(s) for module %s\n", len(modDeps), actualModule)
	}

	dependencies := []DependencyTree{}
	for _, depModule := range modDeps {
		// Circular dependencies are ignored once we encounter a loop
		if isCircularDependency(&depModule, &rootDepPath) {
			if Debug {
				fmt.Printf("WARN: Circular dependency breakout for %s\n", depModule.Path)
			}
			continue
		}

		depPath := append(rootDepPath, *module)
		depSubTree, err := resolveRecursive(&depModule, level+1, depPath, depFlatGraph, actualModuleVersions)
		if err != nil {
			return nil, err
		}

		if Debug {
			fmt.Printf("- Appending dep tree of %s to %s (%s)\n", depModule.Path, modPath, actualModule.Path)
		}

		dependencies = append(dependencies, *depSubTree)
	}

	return &DependencyTree{
		Mod:          &actualModule,
		Dependencies: dependencies,
	}, nil
}

func recomputeDependencyTree(
	rootModule *Module,
	depFlatGraph []ModuleDep,
	actualModuleVersions map[string]Module,
) (*DependencyTree, error) {
	// Main root node
	depTree := DependencyTree{
		Mod: rootModule,
	}

	// Some deps have circular dependencies so we need to break out when
	// we encounter them
	rootDepPath := []Module{*rootModule}

	// Queue up the main root node deps and recurseviley resolve
	rootDeps := getModuleDependencies(depFlatGraph, *rootModule)
	for _, rootDep := range rootDeps {
		module := rootDep
		if Debug {
			fmt.Printf("Resolving root dependency %s\n", module)
		}
		depPath := append(rootDepPath, module)
		depSubtree, err := resolveRecursive(&module, 0, depPath, depFlatGraph, actualModuleVersions)
		if err != nil {
			return nil, err
		}
		depTree.Dependencies = append(depTree.Dependencies, *depSubtree)
	}

	return &depTree, nil
}

// TODO: Actually use `skipDuplicates` value
func printDepTree(buf *bufio.Writer, depTree *DependencyTree, level int, skipDuplicates bool) {
	for idx := 0; idx < level; idx++ {
		io.WriteString(buf, "\t")
	}

	io.WriteString(buf, fmt.Sprintf("- %s@%s\n", depTree.Mod.Path, depTree.Mod.Version))
	for _, depSubtree := range depTree.Dependencies {
		printDepTree(buf, &depSubtree, level+1, skipDuplicates)
	}
}

func main() {
	ctx := context.Background()
	rootModuleName, depFlatGraph, actualModuleVersions, err := getDependencies(ctx)
	if err != nil {
		fmt.Printf("ERROR: %s", err)
		os.Exit(1)
	}

	rootModulePtr := &Module{
		Path:    rootModuleName,
		Version: RootModulePlaceholderVersion,
	}

	fmt.Println("Computing actual dependency tree (this will take a while)...")
	depTree, err := recomputeDependencyTree(rootModulePtr, depFlatGraph, actualModuleVersions)
	if err != nil {
		fmt.Printf("ERROR: %s", err)
		os.Exit(1)
	}

	skipDuplicates := false
	var outputFile *os.File

	if WriteToFile {
		fmt.Printf("Writing output to '%s' (this may take a while)...\n", OutputFileName)
		outputFile, err = os.OpenFile(OutputFileName, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Printf("ERROR: %s", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("Printing dependency tree...")
		fmt.Println("===========================")
		outputFile = os.Stdout
	}

	if WriteToFile {
		defer outputFile.Close()
	}

	writer := bufio.NewWriter(outputFile)
	printDepTree(writer, depTree, 0, skipDuplicates)
	writer.Flush()

	fmt.Println("Done!")
}
