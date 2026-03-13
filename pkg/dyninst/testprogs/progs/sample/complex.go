// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"errors"
	"fmt"
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
type (
	deepPtr1 struct{ a *deepPtr2 }
	deepPtr2 struct{ a *deepPtr3 }
	deepPtr3 struct{ a *deepPtr4 }
	deepPtr4 struct{ a *deepPtr5 }
	deepPtr5 struct{ a *deepPtr6 }
	deepPtr6 struct{ a *deepPtr7 }
	deepPtr7 struct{ a *deepPtr8 }
	deepPtr8 struct{ a *deepPtr9 }
	deepPtr9 struct{ a any }
)

var aDeepPtr1 = deepPtr1{a: &deepPtr2{a: &deepPtr3{a: &deepPtr4{
	a: &deepPtr5{a: &deepPtr6{a: &deepPtr7{a: &deepPtr8{a: &deepPtr9{a: nil}}}}},
}}}}

// Make it circular, just for fun.
func init() { aDeepPtr1.a.a.a.a.a.a.a.a.a = &aDeepPtr1 }

//go:noinline
func testDeepPtr1(a deepPtr1) {}

//go:noinline
func testDeepPtr7(a *deepPtr7) {}

type (
	deepSlice1 struct{ a []*deepSlice2 }
	deepSlice2 struct{ a []*deepSlice3 }
	deepSlice3 struct{ a []*deepSlice4 }
	deepSlice4 struct{ a []*deepSlice5 }
	deepSlice5 struct{ a []*deepSlice6 }
	deepSlice6 struct{ a []*deepSlice7 }
	deepSlice7 struct{ a []*deepSlice8 }
	deepSlice8 struct{ a []*deepSlice9 }
	deepSlice9 struct{ a []any }
)

var aDeepSlice1 = deepSlice1{a: []*deepSlice2{{a: []*deepSlice3{{a: []*deepSlice4{
	{a: []*deepSlice5{{a: []*deepSlice6{{a: []*deepSlice7{{a: []*deepSlice8{{
		a: []*deepSlice9{{}},
	}}}}}}}}},
}}}}}}

func init() { aDeepSlice1.a[0].a[0].a[0].a[0].a[0].a[0].a[0].a[0].a = []any{&aDeepSlice1} }

//go:noinline
func testDeepSlice1(a deepSlice1) {}

//go:noinline
func testDeepSlice7(a *deepSlice7) {}

type (
	deepMap1 struct{ a map[int]*deepMap2 }
	deepMap2 struct{ a map[int]*deepMap3 }
	deepMap3 struct{ a map[int]*deepMap4 }
	deepMap4 struct{ a map[int]*deepMap5 }
	deepMap5 struct{ a map[int]*deepMap6 }
	deepMap6 struct{ a map[int]*deepMap7 }
	deepMap7 struct{ a map[int]*deepMap8 }
	deepMap8 struct{ a map[int]*deepMap9 }
	deepMap9 struct{ a map[int]any }
)

var aDeepMap1 = deepMap1{a: map[int]*deepMap2{
	2: {a: map[int]*deepMap3{
		3: {a: map[int]*deepMap4{
			4: {a: map[int]*deepMap5{
				5: {a: map[int]*deepMap6{
					6: {a: map[int]*deepMap7{
						7: {a: map[int]*deepMap8{
							8: {a: map[int]*deepMap9{
								9: {a: map[int]any{}},
							}},
						}},
					}},
				}},
			}},
		}},
	}},
}}

func init() { aDeepMap1.a[2].a[3].a[4].a[5].a[6].a[7].a[8].a[9].a[1] = &aDeepMap1 }

//go:noinline
func testDeepMap1(a deepMap1) {}

//go:noinline
func testDeepMap7(a *deepMap7) {}

type (
	stringType1 string
	stringType2 string
)

// Test that with a reference depth of 1, we still get the string data for
// the stringType1.
//
//go:noinline
func testStringType1(a *stringType1) {}

// Test that with a reference depth of 1, we do not get the string data for the
// stringType2.
//
//go:noinline
func testStringType2(a **stringType2) {}

//go:noinline
func testLongFunctionWithChangingState() {
	s := 3
	// This variable is going to have different captured value
	// based on the architecture. Give it explicit name for easier
	// workaround using a dedicated snapshot redactor.
	aPerArch := 0
	b := 1
	fmt.Println(s, aPerArch, b)
	// This loop and the following print statement reproduce
	// https://github.com/golang/go/issues/75615
	for range 10 {
		aPerArch, b = b, aPerArch+b
	}
	s += aPerArch
	fmt.Println(s, aPerArch, b)
	for range 10 {
		aPerArch, b = b-aPerArch, aPerArch
	}
	s += b
	fmt.Println(s, aPerArch, b)
}

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

	testDeepPtr1(aDeepPtr1)
	testDeepPtr7(aDeepPtr1.a.a.a.a.a.a)

	testDeepSlice1(aDeepSlice1)
	testDeepSlice7(aDeepSlice1.a[0].a[0].a[0].a[0].a[0].a[0])

	testDeepMap1(aDeepMap1)
	testDeepMap7(aDeepMap1.a[2].a[3].a[4].a[5].a[6].a[7])

	s1 := stringType1("s1")
	s2 := stringType2("s2")
	s2p := &s2
	testStringType1(&s1)
	testStringType2(&s2p)

	testLongFunctionWithChangingState()
}
