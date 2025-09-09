// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package platforms exposes variable with content of platfoms.json file
package platforms

import (
	"fmt"
	"strings"

	"github.com/DataDog/test-infra-definitions/components/os"
)

// BuildOSDescriptor builds an os.Descriptor from "platform/architecture/osVersion"
// e.g. `debian/x86_64/12`
// See all the pinned versions within the platforms.json file of test-infra-definitions.
func BuildOSDescriptor(descriptor string) (os.Descriptor, error) {
	parts := strings.Split(descriptor, "/")
	if len(parts) != 3 {
		return os.Descriptor{}, fmt.Errorf("invalid OS descriptor specification: %s (should be like 'platform/architecture/osVersion')", descriptor)
	}
	platform := parts[0]
	architecture := parts[1]
	osVersion := parts[2]

	return os.NewDescriptorWithArch(os.FlavorFromString(platform), osVersion, os.ArchitectureFromString(architecture)), nil
}

// ParseOSDescriptors parses a comma-separated list of OS descriptors like "platform/architecture/osVersion,..."
func ParseOSDescriptors(descriptors string) ([]os.Descriptor, error) {
	parts := strings.Split(descriptors, ",")
	var descriptorList []os.Descriptor
	for _, descriptor := range parts {
		descriptor, err := BuildOSDescriptor(descriptor)
		if err != nil {
			return nil, err
		}

		descriptorList = append(descriptorList, descriptor)
	}

	return descriptorList, nil
}
