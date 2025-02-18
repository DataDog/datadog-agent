// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package internal contains internal logic for the fleet package.
package internal

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoOutsideImport(t *testing.T) {
	// Root directory to start the walk
	rootDir := "."

	// Define the unwanted import path
	datadogAgentPrefix := "github.com/DataDog/datadog-agent/"
	allowedPaths := []string{
		"pkg/fleet/installer", // TODO: cleanup & remove
		"pkg/fleet/internal",
	}

	// Walk the directory tree
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only check .go files
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".go") {
			// Create a file set and parse the file
			fs := token.NewFileSet()
			node, err := parser.ParseFile(fs, path, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("failed to parse file: %v", err)
			}

			// Loop through the imports in the AST
			for _, imp := range node.Imports {
				// Check if the import path matches the unwanted import
				isAllowedImport := true
				if strings.HasPrefix(imp.Path.Value, "\""+datadogAgentPrefix) {
					isAllowedImport = false
					for _, allowedPath := range allowedPaths {
						if strings.HasPrefix(imp.Path.Value, "\""+datadogAgentPrefix+allowedPath) {
							isAllowedImport = true
						}
					}
				}
				if !isAllowedImport {
					t.Errorf("file %s imports %s, which is not allowed", path, imp.Path.Value)
				}
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk directory: %v", err)
	}
}
