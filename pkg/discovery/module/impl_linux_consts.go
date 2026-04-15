// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

// maxNumberOfPorts is the maximum number of listening ports which we report per
// service.
const maxNumberOfPorts = 50
