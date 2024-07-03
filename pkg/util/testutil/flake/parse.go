// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package flake

import (
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

const flakyTestIndicator = "flakytest: this is a known flaky test"

// KnownFlakyTests is data about which tests should ignore failures, because they are flaky
type KnownFlakyTests struct {
	// map between a full package name to a set of test names
	packageTestList map[string]map[string]struct{}
}

// Add adds a flaky test to the list of known flaky tests
func (k *KnownFlakyTests) Add(pkg string, testName string) {
	if k.packageTestList == nil {
		k.packageTestList = make(map[string]map[string]struct{})
	}
	if _, ok := k.packageTestList[pkg]; !ok {
		k.packageTestList[pkg] = make(map[string]struct{})
	}
	k.packageTestList[pkg][testName] = struct{}{}
}

// IsFlaky returns whether the particular test, or its parents, is known as flaky
func (k *KnownFlakyTests) IsFlaky(pkg string, testName string) bool {
	if _, ok := k.packageTestList[pkg]; !ok {
		return false
	}
	if _, ok := k.packageTestList[pkg][testName]; ok {
		return true
	}

	// check parents of test (if any)
	parentTest := testName
	for idx := strings.LastIndex(parentTest, "/"); idx != -1; idx = strings.LastIndex(parentTest, "/") {
		parentTest = parentTest[:idx]
		if _, ok := k.packageTestList[pkg][parentTest]; ok {
			return true
		}
	}

	return false
}

// Parse parses the reader in the flake.yaml format
func Parse(r io.Reader) (*KnownFlakyTests, error) {
	dec := yaml.NewDecoder(r)
	pkgToTests := make(map[string][]string)
	if err := dec.Decode(&pkgToTests); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	kf := &KnownFlakyTests{packageTestList: make(map[string]map[string]struct{})}
	for pkg, tests := range pkgToTests {
		for _, t := range tests {
			kf.Add(pkg, t)
		}
	}
	return kf, nil
}

// HasFlakyTestMarker returns whether the output string contains the flaky test indicator
func HasFlakyTestMarker(out string) bool {
	return strings.Contains(out, flakyTestIndicator)
}
