// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

// FieldTypes tells the translator which SECL fields hold several values.
//
// That is the one thing the syntax does not say and the translation cannot do
// without: SECL reads a comparison between an array field and a scalar as "any
// element matches", so `process.ancestors.file.name == "sh"` and
// `process.ancestors.file.name in [ "sh" ]` both mean an existential quantifier
// that has to be written out in CEL.
type FieldTypes interface {
	// ListPrefix returns the longest prefix of the field that is an iterated
	// node, e.g. "process.ancestors" for "process.ancestors.file.name". It
	// returns an empty string when nothing along the path is iterated.
	ListPrefix(field string) string

	// IsListLeaf reports whether the field itself holds several values, as
	// process.argv does, without anything along its path being iterated.
	IsListLeaf(field string) bool

	// IsPseudoField reports whether the field is one of the `length` and
	// `root_domain` pseudo fields, which SECL derives from another field rather
	// than storing. They are translated to size() and a helper call.
	IsPseudoField(field string) bool
}
