// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ast

// Ident is an identifier: `O_RDONLY`, `my_macro`, `process` in
// `process.file.name`.
//
// It may name an event field, a constant or a macro.
type Ident struct {
	Meta

	Name string
}

// Selector is a dot separated name: `process.file`, `process.file.name`.
//
// Its position is that of the identifier the name starts with.
type Selector struct {
	Meta

	// Base is the Selector `process.file` in `process.file.name`.
	Base Node
	// Member is `name` in `process.file.name`.
	Member string
}

// Index is a name with a subscript: `process.ancestors[A]`, `process.argv[0]`.
//
// Its position is that of the identifier the name starts with.
type Index struct {
	Meta

	// Base is the Selector `process.ancestors` in `process.ancestors[A]`.
	Base Node
	// Subscript is `A` in `process.ancestors[A]`.
	Subscript Node
}

// VariableRef is a reference to a SECL variable: `${my_var}`.
type VariableRef struct {
	Meta

	Name string
}

// The types below are produced by lowering passes rather than parsing: a name
// is lowered into a reference to the field it refers to.

// FieldRef is a reference to an event field: `process.file.name`,
// `process.ancestors[A].file.name`. An index in an array field,
// `process.argv[0]`, is an Index whose Base is the reference to the array.
type FieldRef struct {
	Meta

	// Name is the name of the field, without its subscript:
	// `process.ancestors.file.name`.
	Name string
	// Iter is the iteration the field is read in, nil if it isn't iterated over
	// with a variable.
	Iter *Iteration
}

// Iteration is the iteration of a field with an iterator variable:
// `process.ancestors[A]` in `process.ancestors[A].file.name`. The fields
// iterated over with the same variable are read from the same element.
type Iteration struct {
	// Iterator is the field iterated over, the name before the subscript:
	// `process.ancestors`.
	Iterator string
	// Register is the variable: `A`.
	Register string
}
