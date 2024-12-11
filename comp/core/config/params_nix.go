// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build netbsd || openbsd || solaris || dragonfly || linux

package config

const (
	// DefaultConfPath points to the folder containing datadog.yaml
	DefaultConfPath = "/etc/datadog-agent"
)
