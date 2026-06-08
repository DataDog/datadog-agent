// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive
package sds

type Match struct {
	RuleID string
	Path   string
}

type RuleDefinition struct {
	ID       string
	Name     string
	Regex    string
	Priority string
	Tags     []string
	Labels   []string
}
