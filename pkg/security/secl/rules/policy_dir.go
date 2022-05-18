// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"os"
	"path/filepath"
	"sort"
)

// GetFileProviders returns providers for the given policies dir
func GetFileProviders(policiesDir string) ([]PolicyProvider, error) {
	policyFiles, err := os.ReadDir(policiesDir)
	if err != nil {
		return nil, &ErrPoliciesLoad{Name: policiesDir, Err: err}
	}
	sort.Slice(policyFiles, func(i, j int) bool {
		switch {
		case policyFiles[i].Name() == defaultPolicyFile:
			return true
		case policyFiles[j].Name() == defaultPolicyFile:
			return false
		default:
			return policyFiles[i].Name() < policyFiles[j].Name()
		}
	})

	var providers []PolicyProvider

	// Load and parse policies
	for _, policyPath := range policyFiles {
		name := policyPath.Name()

		// policy path extension check
		if filepath.Ext(name) != ".policy" {
			continue
		}

		filename := filepath.Join(policiesDir, name)
		providers = append(providers, NewPolicyFileProvider(filename))
	}

	return providers, nil
}
