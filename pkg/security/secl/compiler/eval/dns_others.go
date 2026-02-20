// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Package eval holds eval related files
package eval

import "github.com/alecthomas/participle/lexer"

func evaluatorFromRootDomainHandler(_ string, pos lexer.Position, _ *State) (interface{}, lexer.Position, error) {
	return nil, pos, NewError(pos, "handler not implemented for this platform")
}
