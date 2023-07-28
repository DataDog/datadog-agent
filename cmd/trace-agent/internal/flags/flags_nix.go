// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package flags

// DefaultConfigPath specifies the default configuration file path for non-Windows systems.
const DefaultConfigPath = "/opt/datadog-agent/etc/datadog.yaml"

func registerOSSpecificFlags() {}
