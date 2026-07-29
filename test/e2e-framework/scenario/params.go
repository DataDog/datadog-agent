// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import "fmt"

// NewParams returns a pointer to a T with all schema defaults applied. It is the
// blessed constructor for scenario params in Go code: it yields exactly the
// values the CLI/service produce for the same (empty) input, so Go tests and the
// CLI never drift. Panics if T is not a valid params struct (an authoring error).
func NewParams[T any]() *T {
	p := new(T)
	s, err := BuildSchema(p)
	if err != nil {
		panic(fmt.Sprintf("scenario.NewParams[%T]: %v", *p, err))
	}
	if err := ApplyDefaults(s, p); err != nil {
		panic(fmt.Sprintf("scenario.NewParams[%T]: %v", *p, err))
	}
	return p
}
