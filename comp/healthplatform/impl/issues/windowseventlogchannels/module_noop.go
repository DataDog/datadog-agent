// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !windows

// Package windowseventlogchannels provides an issue module for Windows Event Log channel misconfiguration.
// It detects configured channel_path values in win32_event_log integrations that do not exist on the host.
package windowseventlogchannels
