// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !docker

package rofspermissions

import "github.com/DataDog/datadog-agent/pkg/util/fxutil"

// Module provides no issue modules when Docker support is not built.
func Module() fxutil.Module { return fxutil.Component() }
