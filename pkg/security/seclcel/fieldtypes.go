// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

// FieldTypes tells the translator what it cannot read off the syntax: which
// names are fields, and which of them hold several values.
//
// The second is the one the translation cannot do without: SECL reads a
// comparison between an array field and a scalar as "any element matches", so
// `process.ancestors.file.name == "sh"` and
// `process.ancestors.file.name in [ "sh" ]` both mean an existential quantifier
// that has to be written out in CEL.
type FieldTypes interface {
	// IsFieldRoot reports whether name is a top level segment of the field
	// namespace, and so whether a name starting with it is a field rather than a
	// macro or a constant. Only a field is rooted at EventRoot.
	IsFieldRoot(name string) bool

	// Canonical returns the name a field is known by now, for the names a rule may
	// still be written with. It returns its argument unchanged for everything else,
	// including a macro or a constant.
	Canonical(field string) string

	// ListPrefix returns the longest prefix of the field that is an iterated
	// node, e.g. "process.ancestors" for "process.ancestors.file.name". It
	// returns an empty string when nothing along the path is iterated.
	ListPrefix(field string) string

	// IsListLeaf reports whether the field itself holds several values, as
	// process.argv does, without anything along its path being iterated.
	IsListLeaf(field string) bool

	// GlobPattern reports whether a `~"…"` pattern compared against the field is
	// compiled as a glob rather than as a pattern: `*` stopping at a path separator,
	// `**` allowed. SECL decides that per field, through an operator override that
	// rewrites the value type of whatever the field is compared against, so the
	// translation has to ask the same question wherever it turns a pattern into a
	// call.
	GlobPattern(field string) bool

	// IsPseudoField reports whether the field is one of the `length` and
	// `root_domain` pseudo fields, which SECL derives from another field rather
	// than storing. They are translated to size() and a helper call.
	IsPseudoField(field string) bool
}
