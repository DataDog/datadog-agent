// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package util

import "github.com/DataDog/datadog-agent/pkg/logs/util/testutils"

// CreateSources creates sources
// TODO: This alias will be removed once logs agent module refactor is complete.
var CreateSources = testutils.CreateSources
