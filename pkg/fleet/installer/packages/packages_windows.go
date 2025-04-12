// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

var (
	// Packages is a map of package names to their implementations
	Packages = map[string]Package{
		apmLibraryDotnetPackage.name: apmLibraryDotnetPackage,
	}
)
