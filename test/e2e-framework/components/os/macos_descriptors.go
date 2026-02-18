// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package os

// Implements commonly used descriptors for easier usage
// See platforms.go for the AMIs used for each OS
var (
	MacOSDefault = MacOSSonoma
	MacOSSonoma  = NewDescriptor(MacosOS, "sonoma")
)

var MacOSDescriptorsDefault = map[Flavor]Descriptor{
	MacosOS: MacOSDefault,
}
