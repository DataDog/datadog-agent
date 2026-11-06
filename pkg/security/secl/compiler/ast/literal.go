// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ast

// Ident is a bare identifier: `process.file.name`, `O_RDONLY`, `my_macro`,
// `process.ancestors[A].file.name`.
//
// It may name an event field, a constant, a macro or an iterator.
type Ident struct {
	Meta

	Name string
}

// Literal is a literal value: `true`, `123`, `10s`, `"/etc/passwd"`,
// `~"/etc/*"`, `r"/etc/.*"`, `127.0.0.1`, `192.168.0.0/24`.
type Literal struct {
	Meta

	Value Value
}

// VariableRef is a reference to a SECL variable: `${my_var}`.
type VariableRef struct {
	Meta

	Name string
}

// FieldRef is an explicit reference to an event field: `%{process.pid}`.
//
// Unlike an Ident, it never names a constant or a macro.
type FieldRef struct {
	Meta

	Name string
}

// Array is an array literal: `[1, 2]`, `["/etc/passwd", ~"/etc/*"]`,
// `[127.0.0.1, 192.168.0.0/24]`.
//
// Its members are of the same family of kinds: booleans; integers and durations;
// strings, patterns and regexps; IPs and CIDRs.
type Array struct {
	Meta

	Members []*Literal
}

// IdentArray is an array literal of identifiers: `[O_RDONLY, O_WRONLY]`.
//
// Only constants are supported, and all of them must be of the same type.
type IdentArray struct {
	Meta

	Names []string
}
