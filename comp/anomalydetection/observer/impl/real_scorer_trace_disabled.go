// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !anomalydetectiontestbench

package observerimpl

func researchTraceActive() bool                         { return false }
func researchTraceEnabled(int64) bool                   { return false }
func researchTrace(string, int64, map[string]any)       {}
func researchTraceGlobal(string, int64, map[string]any) {}
