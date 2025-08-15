// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"errors"
	"io"
)

type tierA struct {
	a int
	b tierB
}

type tierB struct {
	c int
	d tierC
}

type tierC struct {
	e int
	f tierD
}

type tierD struct {
	g int
}

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

type interfaceComplexityA struct {
	b int
	c interfaceComplexityB
}

type interfaceComplexityB struct {
	d int
	e error
	f interfaceComplexityC
}

type interfaceComplexityC struct {
	g int
}

//go:noinline
//nolint:all
func testInterfaceComplexity(a interfaceComplexityA) {}

//go:noinline
//nolint:all
func testMultipleStructTiers(a tierA) {}

//nolint:all
//go:noinline
func testMultipleDereferences(o outer) {}

type bigStruct struct {
	x      []*string
	z      int
	writer io.Writer
}

//nolint:all
//go:noinline
func testBigStruct(b bigStruct) {}

type circularReferenceType struct {
	t *circularReferenceType
}

//nolint:all
//go:noinline
func testCircularType(x circularReferenceType) {}

//nolint:all
//go:noinline
func testInterfaceAndInt(a int, b error, c uint) {}

//nolint:all
func executeComplexFuncs() {
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

	testBigStruct(bigStruct{
		x:      s,
		z:      5,
		writer: io.Discard,
	})

	testMultipleStructTiers(tierA{
		a: 1,
		b: tierB{
			c: 2,
			d: tierC{
				e: 3, f: tierD{
					g: 4,
				},
			},
		},
	})
	testMultipleDereferences(o)

	circ := circularReferenceType{}
	circ.t = &circ
	testCircularType(circ)

	testInterfaceComplexity(interfaceComplexityA{
		b: 1,
		c: interfaceComplexityB{
			d: 2,
			e: errors.New("three"),
			f: interfaceComplexityC{
				g: 4,
			},
		},
	})

	testInterfaceAndInt(1, errors.New("two"), 3)

}
