// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package exprfilter

import (
	"testing"

	"github.com/expr-lang/expr"
)

// Container is the sample struct used in expressions
type Container struct {
	Name  string
	Image string
	ID    string
	Pod   *Pod
}

type Pod struct {
	Name        string
	Namespace   string
	Annotations map[string]string
	Labels      map[string]string
}

// Env provides a static type for compile-time checking
type Env struct {
	Container Container
}

// BenchmarkExprRegexMatch tests regex performance in expr
func BenchmarkExprRegexMatch(b *testing.B) {
	// Define a compiled expression
	program, err := expr.Compile(
		`Container.Name matches "nginx.*" && Container.Pod.Namespace == "default" && Container.Pod.Annotations["annotation1"] == "value1"`,
		expr.Env(Env{}), // enable static typing
		expr.AsBool(),   // expect a boolean output
	)
	if err != nil {
		b.Fatalf("compile failed: %v", err)
	}

	// Define the input environment
	input := Env{
		Container: Container{
			Name: "nginx-123",
			Pod: &Pod{
				Name:      "nginx-pod",
				Namespace: "default",
				Annotations: map[string]string{
					"annotation1": "value1",
					"annotation2": "value2",
				},
			},
		},
	}

	// Reset timer to exclude setup time
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := expr.Run(program, input)
		if err != nil {
			b.Fatalf("run failed: %v", err)
		}
	}
}
