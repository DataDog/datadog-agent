// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build jetson || (clusterchecks && !test)

// Package invalidconfig is an empty stub keeping the package importable on IoT
// Agent (jetson) and Cluster Agent builds where the real validator is excluded
// to stay under the binary size budget.
package invalidconfig
