// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ast holds the abstract syntax tree of the SECL language.
package ast

// Ident is a bare identifier: `process.file.name`, `O_RDONLY`, `my_macro`,
// `process.ancestors[A].file.name`.
//
// What an identifier refers to, an event field, a constant, a macro or an
// iterator, is only known once the tree is compiled against a model.
type Ident struct {
	Meta

	// Name is the identifier, subscript included.
	Name string
}

// IntLiteral is an integer literal: `123`.
type IntLiteral struct {
	Meta

	Value int
}

// DurationLiteral is a duration literal: `10s`.
type DurationLiteral struct {
	Meta

	// Value is the duration in nanoseconds.
	Value int
}

// StringLiteral is a string literal: `"/etc/passwd"`.
type StringLiteral struct {
	Meta

	// Value is the content of the literal, unquoted. It may hold `${variable}`
	// and `%{field}` interpolations.
	Value string
}

// PatternLiteral is a glob pattern literal: `~"/etc/*"`.
type PatternLiteral struct {
	Meta

	// Value is the content of the literal, unquoted.
	Value string
}

// RegexpLiteral is a regular expression literal: `r"/etc/.*"`.
type RegexpLiteral struct {
	Meta

	// Value is the content of the literal, unquoted.
	Value string
}

// IPLiteral is an IP literal: `127.0.0.1`.
type IPLiteral struct {
	Meta

	// Value is the textual representation of the IP.
	Value string
}

// CIDRLiteral is a CIDR literal: `192.168.0.0/24`.
type CIDRLiteral struct {
	Meta

	// Value is the textual representation of the CIDR.
	Value string
}

// VariableRef is a reference to a SECL variable: `${my_var}`.
type VariableRef struct {
	Meta

	// Name is the name of the variable, without the `${}` delimiters.
	Name string
}

// FieldRef is an explicit reference to an event field: `%{process.pid}`.
//
// Unlike an Ident, a field reference is never resolved as a constant or as a
// macro.
type FieldRef struct {
	Meta

	// Name is the name of the field, without the `%{}` delimiters.
	Name string
}

// StringArray is an array literal of strings, patterns and regexps:
// `["/etc/passwd", ~"/etc/*", r"/etc/.*"]`.
type StringArray struct {
	Meta

	Members []StringArrayMember
}

// StringArrayMember is a member of a StringArray: a *StringLiteral, a
// *PatternLiteral or a *RegexpLiteral.
type StringArrayMember interface {
	Node

	// stringArrayMember closes the union.
	stringArrayMember()
}

func (*StringLiteral) stringArrayMember()  {}
func (*PatternLiteral) stringArrayMember() {}
func (*RegexpLiteral) stringArrayMember()  {}

// CIDRArray is an array literal of IPs and CIDRs: `[127.0.0.1, 192.168.0.0/24]`.
type CIDRArray struct {
	Meta

	Members []CIDRArrayMember
}

// CIDRArrayMember is a member of a CIDRArray: an *IPLiteral or a *CIDRLiteral.
type CIDRArrayMember interface {
	Node

	// cidrArrayMember closes the union.
	cidrArrayMember()
}

func (*IPLiteral) cidrArrayMember()   {}
func (*CIDRLiteral) cidrArrayMember() {}

// IntArray is an array literal of integers: `[1, 2, 3]`.
type IntArray struct {
	Meta

	Values []int
}

// IdentArray is an array literal of identifiers: `[O_RDONLY, O_WRONLY]`.
//
// Only constants are supported, and all of them must be of the same type.
type IdentArray struct {
	Meta

	Names []string
}
