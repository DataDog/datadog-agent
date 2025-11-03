// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package platforms exposes variable with content of platfoms.json file
package platforms

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
)

// BuildOSDescriptor builds an os.Descriptor from "platform/architecture/osVersion"
// e.g. `debian/x86_64/12`
// See all the pinned versions within the platforms.json file of test-infra-definitions.
func BuildOSDescriptor(descriptor string) (os.Descriptor, error) {
	platform, architecture, osVersion, err := ParseRawOsDescriptor(descriptor)
	if err != nil {
		return os.Descriptor{}, err
	}

	return os.NewDescriptorWithArch(os.FlavorFromString(platform), osVersion, os.ArchitectureFromString(architecture)), nil
}

// ParseRawOsDescriptor parses a raw OS descriptor like "platform/architecture/osVersion"
func ParseRawOsDescriptor(descriptor string) (string, string, string, error) {
	parts := strings.Split(descriptor, "/")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid OS descriptor specification: %s (should be like 'platform/architecture/osVersion')", descriptor)
	}
	platform := parts[0]
	architecture := parts[1]
	osVersion := parts[2]

	return platform, architecture, osVersion, nil
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

// PrettifyOsDescriptor prettifies an os.Descriptor to a string that may be used in a stack name
func PrettifyOsDescriptor(descriptor os.Descriptor) string {
	pretty := fmt.Sprintf("%s-%s-%s", descriptor.Flavor, descriptor.Architecture, descriptor.Version)
	pretty = strings.ReplaceAll(pretty, ".", "-")
	pretty = strings.ReplaceAll(pretty, "_", "-")
	pretty = strings.ReplaceAll(pretty, ":", "-")
	pretty = strings.ReplaceAll(pretty, "/", "-")
	pretty = strings.ReplaceAll(pretty, " ", "-")
	return pretty
}
