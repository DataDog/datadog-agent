// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sample

import "io"

type outer struct {
	A *middle
}

type middle struct {
	B *inner
}

type inner struct {
	C int
	D byte
	E string
}

//nolint:all
//go:noinline
func test_multiple_dereferences(o outer) {}

type bigStruct struct {
	x      []*string
	z      int
	writer io.Writer
}

//nolint:all
//go:noinline
func test_big_struct(b bigStruct) {}

type circularReferenceType struct {
	t *circularReferenceType
}

//nolint:all
//go:noinline
func test_circular_type(x circularReferenceType) {}

//nolint:all
func ExecuteComplexFuncs() {
	o := outer{
		A: &middle{
			B: &inner{
				C: 1,
				D: 2,
				E: "three",
			},
		},
	}

	str := "abc"
	s := []*string{&str}

	test_big_struct(bigStruct{
		x:      s,
		z:      5,
		writer: io.Discard,
	})
	test_multiple_dereferences(o)

	circ := circularReferenceType{}
	circ.t = &circ
	test_circular_type(circ)
}
