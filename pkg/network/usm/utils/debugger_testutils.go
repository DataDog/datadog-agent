// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

package utils

// GetTracedPrograms returns a list of traced programs by the specific program type
func GetTracedPrograms(programType string) []TracedProgram {
	res := debugger.GetTracedPrograms()
	i := 0 // output index
	for _, x := range res {
		if x.ProgramType == programType {
			// copy and increment index
			res[i] = x
			i++
		}
	}
	return res[:i]
}
