// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package goversion provides a wrapper around the `GoVersion` type from the
// delve debugger
package goversion

import (
	"fmt"

	"github.com/go-delve/delve/pkg/goversion"
)

// GoVersion is just a wrapper around go-delve's goversion.GoVersion that also
// preserves the raw version string used for instantiating it. This is movivated
// by the need to disambiguate versions without revision numbers from versions
// with revision number 0 (eg. `1.19.0` vs `1.19`).
type GoVersion struct {
	goversion.GoVersion
	rawVersion string
}

// NewGoVersion returns a new GoVersion struct
func NewGoVersion(rawVersion string) (GoVersion, error) {
	version, ok := goversion.Parse(fmt.Sprintf("go%s", rawVersion))
	if !ok {
		return GoVersion{}, fmt.Errorf("couldn't parse go version %s", rawVersion)
	}

	return GoVersion{
		GoVersion:  version,
		rawVersion: rawVersion,
	}, nil
}

// AfterOrEqual returns whether one GoVersion is after or
// equal to the other.
func (v *GoVersion) AfterOrEqual(other GoVersion) bool {
	return v.GoVersion.AfterOrEqual(other.GoVersion)
}

// String returns the raw version string from this GoVersion
func (v GoVersion) String() string {
	return v.rawVersion
}
