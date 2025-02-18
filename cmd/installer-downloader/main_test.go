// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package main

import (
	"os"
	"testing"
)

func TestFlavorPaths(t *testing.T) {
	for _, path := range flavorPaths {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("file %q does not exist", path)
		} else if err != nil {
			t.Errorf("error checking file %q: %v", path, err)
		}
	}
}
