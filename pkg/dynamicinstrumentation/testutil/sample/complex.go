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

type big_struct struct {
	x []*string
	z int
	io.Writer
}

//nolint:all
//go:noinline
func test_big_struct(b big_struct) {}

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
	test_big_struct(big_struct{})
	test_multiple_dereferences(o)
}
