// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package collectors is a wrapper that loads the collectors which should run
// only in the core agent
package collectors

import (
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/remote/process_collector" // Load the collectors
)
