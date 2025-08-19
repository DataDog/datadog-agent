// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package main contains the logic for the go.mod file parser
package main

import (
	"flag"
	"fmt"
	"os"
	"slices"

	"golang.org/x/mod/modfile"

	"gopkg.in/yaml.v3"
)

func parseWorkfile(path string) (*modfile.WorkFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read %s file", path)
	}

	parsedFile, err := modfile.ParseWork(path, data, nil)
	if err != nil {
		return nil, fmt.Errorf("could not parse %s file", path)
	}

	return parsedFile, nil
}

type modules struct {
	Modules map[string]any `yaml:"modules"`
}

func parseModulesList(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read %s file", path)
	}

	var parsedModules modules
	err = yaml.Unmarshal(data, &parsedModules)
	if err != nil {
		return nil, fmt.Errorf("could not parse %s file", path)
	}

	res := make([]string, 0, len(parsedModules.Modules))
	for module, moduleConfig := range parsedModules.Modules {
		if config, ok := moduleConfig.(string); ok && config == "ignored" {
			continue
		}
		res = append(res, module)
	}
	return res, nil
}
func main() {
	var workPath string
	var modulesFilePath string

	flag.StringVar(&workPath, "path", "", "Path to the go module to inspect")
	flag.StringVar(&modulesFilePath, "modules-file", "", "Path to modules.yml file")

	flag.Parse()

	// Check that both flags have been set
	if flag.NFlag() != 2 {
		flag.Usage()
		os.Exit(1)
	}

	parsedWorkFile, err := parseWorkfile(workPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	parsedModules, err := parseModulesList(modulesFilePath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	slices.Sort(parsedModules)

	parsedWorkFile.SetUse([]*modfile.Use{})
	for _, module := range parsedModules {
		parsedWorkFile.AddUse(module, module)
	}

	if err := os.WriteFile(workPath, modfile.Format(parsedWorkFile.Syntax), 0644); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

}
