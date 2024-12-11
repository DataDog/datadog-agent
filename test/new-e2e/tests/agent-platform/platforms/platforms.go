// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package platforms exposes variable with content of platfoms.json file
package platforms

import (
	_ "embed"
	"strings"

	"github.com/DataDog/test-infra-definitions/components/os"
)

// Content of the platforms.json file
//
//go:embed platforms.json
var Content []byte

// BuildOSDescriptor builds an os.Descriptor from a platform, architecture and osVersion
func BuildOSDescriptor(platform, architecture, osVersion string) os.Descriptor {
	// For some reason, centos is mixing multiple os with different users in AMIs in `platforms.json`
	// Performing remapping to have the right user for AMI
	if platform == "centos" {
		switch {
		case strings.Contains(osVersion, "rhel"):
			platform = "redhat"

		case strings.Contains(osVersion, "rocky"):
			platform = "rockylinux"
		}
	}

	return os.NewDescriptorWithArch(os.FlavorFromString(platform), osVersion, os.ArchitectureFromString(architecture))
}
