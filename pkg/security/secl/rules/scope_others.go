// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Package rules holds rules related files
package rules

import "github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"

// VariableScopes is the list of scopes for variables
var VariableScopes = []string{
	ScopeProcess,
	ScopeContainer,
}

// // DefaultStateScopes returns the default state scopes for variables
// func DefaultStateScopes() map[string]eval.VariableScoper {
// 	return getCommonStateScopes()
// }

// DefaultVariableScopers returns the default variable scopers
func DefaultVariableScopers() map[Scope]*eval.VariableScoper {
	return getCommonVariableScopers()
}
