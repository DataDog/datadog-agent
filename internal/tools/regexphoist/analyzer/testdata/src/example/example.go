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

func patterns(dynamic string) {
	re.MustCompile("x")       // want "regexp.MustCompile with a constant pattern should be a package-level var so it compiles once, not on every call"
	re.MustCompile(pattern)   // want "regexp.MustCompile with a constant pattern"
	re.MustCompile("x" + "y") // want "regexp.MustCompile with a constant pattern"
	re.MustCompilePOSIX("x")  // want "regexp.MustCompile with a constant pattern"
	re.MustCompile(dynamic)
	re.MustCompile("x" + dynamic)
	variable := "x"
	re.MustCompile(variable)
	re.MustCompilePOSIX(dynamic)
	re.Compile("x")
	func() {
		re.MustCompile("nested") // want "regexp.MustCompile with a constant pattern"
	}()
}

type receiver struct{}

func (receiver) method() {
	re.MustCompile("method") // want "regexp.MustCompile with a constant pattern"
}

func shadowed() {
	re := struct{ MustCompile func(string) }{MustCompile: func(string) {}}
	re.MustCompile("x")
}
