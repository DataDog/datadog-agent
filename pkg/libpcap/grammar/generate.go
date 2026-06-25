// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package grammar implements the BPF filter expression parser and lexer.
package grammar

//go:generate goyacc -o grammar.go -p yy grammar.y
