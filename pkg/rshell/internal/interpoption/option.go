// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package interpoption provides internal-only interpreter options that are not
// part of the public API. External consumers of the module cannot import this
// package due to Go's internal/ directory convention.
package interpoption

// AllowAllCommands returns a value of type interp.RunnerOption that permits
// execution of any command (builtin or external), bypassing the AllowedCommands
// restriction. It is populated by interp.init().
//
// Callers must type-assert the result:
//
//	opt := interpoption.AllowAllCommands().(interp.RunnerOption)
var AllowAllCommands func() any
