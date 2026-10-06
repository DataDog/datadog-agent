// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package example

import re "regexp"

const pattern = "x"

var packageRegexp = re.MustCompile("x")
var packagePOSIX = re.MustCompilePOSIX(pattern)
var packageInitializer = func() *re.Regexp {
	return re.MustCompile("x")
}()

func patterns(dynamic string, n int, xs []string) {
	re.MustCompile("x")
	re.MustCompile(pattern)
	re.MustCompile("x" + "y")
	re.MustCompilePOSIX("x")
	for i := 0; i < n; i++ {
		re.MustCompile("x")       // want "regexp.MustCompile with a constant pattern inside a loop should be hoisted to a package-level var so it compiles once, not on every iteration"
		re.MustCompile(pattern)   // want "regexp.MustCompile with a constant pattern inside a loop"
		re.MustCompile("x" + "y") // want "regexp.MustCompile with a constant pattern inside a loop"
		re.MustCompilePOSIX("x")  // want "regexp.MustCompile with a constant pattern inside a loop"
		re.MustCompile(dynamic)
		re.MustCompile("x" + dynamic)
		variable := "x"
		re.MustCompile(variable)
		re.MustCompilePOSIX(dynamic)
		re.Compile("x")
		func() {
			re.MustCompile("closure")
			for range xs {
				re.MustCompile("closure-loop") // want "regexp.MustCompile with a constant pattern inside a loop"
			}
		}()
		for range xs {
			if n > 0 {
				re.MustCompile("nested") // want "regexp.MustCompile with a constant pattern inside a loop"
			}
		}
		re.MustCompile("after-closure") // want "regexp.MustCompile with a constant pattern inside a loop"
	}
	for range xs {
		re.MustCompile("range") // want "regexp.MustCompile with a constant pattern inside a loop"
	}
	re.MustCompile("after-loop")
	for r := re.MustCompile("init"); r.MatchString(dynamic); {
		break
	}
	for re.MustCompile("condition").MatchString(dynamic) {
		break
	}
	for range re.MustCompile("range-expression").FindAllString(dynamic, -1) {
	}
}

type receiver struct{}

func (receiver) method() {
	re.MustCompile("method")
	for {
		re.MustCompile("method-loop") // want "regexp.MustCompile with a constant pattern inside a loop"
		break
	}
}

func shadowed() {
	re := struct{ MustCompile func(string) }{MustCompile: func(string) {}}
	for {
		re.MustCompile("x")
		break
	}
}
