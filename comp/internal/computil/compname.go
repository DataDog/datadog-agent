// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package computil implements some utilities for defining components.
package computil

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// GetComponentName gets the component name of the caller.  This must be called
// from a source file in a package of the form
// `github.com/DataDog/datadog-agent/comp/<bundle>` or
// `github.com/DataDog/datadog-agent/comp/<bundle>/module`.
func GetComponentName() string {
	_, filename, _, ok := runtime.Caller(1)
	if !ok {
		panic("cannot determine component name")
	}
	filename = filepath.ToSlash(filename)
	components := strings.Split(filename, "/")
	if len(components) >= 3 && components[len(components)-3] == "comp" {
		// a bundle
		return fmt.Sprintf("comp/%s", components[len(components)-2])
	}
	if len(components) >= 4 && components[len(components)-4] == "comp" {
		// a component
		return fmt.Sprintf("comp/%s/%s", components[len(components)-3], components[len(components)-2])
	}
	panic("must be called from a component or a bundle")
}
