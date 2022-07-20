// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostname

import "github.com/DataDog/datadog-agent/pkg/util/hostname/internal/file"

import "github.com/DataDog/datadog-agent/pkg/traceinit"

func init() {
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\util\hostname\file.go 10`)
	RegisterHostnameProvider("file", file.HostnameProvider)
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\util\hostname\file.go 11`)
}