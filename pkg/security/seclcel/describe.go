// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
)

// DescribeEnv renders the fields an environment declares, in CEL's own type
// notation.
//
// It reads the environment through cel-go's interfaces rather than the generated
// tables, so it shows what a rule can actually refer to — including anything a
// caller declared with extra options — and cannot drift from the declarations it
// describes.
//
// CEL has no textual form for *declaring* object types, so this is a description
// and not something that can be loaded back. The loadable part of an environment,
// its variables and functions, can be had from cel.Env.ToConfig.
func DescribeEnv(env *cel.Env) string {
	provider := env.CELTypeProvider()

	variables := env.Variables()
	sort.Slice(variables, func(i, j int) bool { return variables[i].Name() < variables[j].Name() })

	// Every reachable object type, found by following the declared variables.
	// types.Provider cannot enumerate its types, but it does not need to: a type
	// no variable leads to is one no expression can name.
	reachable := map[string]bool{}
	var pending []string
	for _, variable := range variables {
		pending = append(pending, objectTypesIn(variable.Type())...)
	}

	for len(pending) > 0 {
		name := pending[0]
		pending = pending[1:]
		if reachable[name] {
			continue
		}
		reachable[name] = true

		for _, member := range memberNames(provider, name) {
			fieldType, ok := provider.FindStructFieldType(name, member)
			if !ok {
				continue
			}
			pending = append(pending, objectTypesIn(fieldType.Type)...)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %d variables, %d types\n\n", len(variables), len(reachable))

	for _, variable := range variables {
		fmt.Fprintf(&b, "%s: %s\n", variable.Name(), variable.Type())
	}

	names := make([]string, 0, len(reachable))
	for name := range reachable {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		fmt.Fprintf(&b, "\n%s {\n", name)
		for _, member := range memberNames(provider, name) {
			fieldType, ok := provider.FindStructFieldType(name, member)
			if !ok {
				continue
			}
			fmt.Fprintf(&b, "\t%s: %s\n", member, fieldType.Type)
		}
		b.WriteString("}\n")
	}

	return b.String()
}

// memberNames returns a type's members in a stable order.
func memberNames(provider types.Provider, structType string) []string {
	members, ok := provider.FindStructFieldNames(structType)
	if !ok {
		return nil
	}
	sort.Strings(members)
	return members
}

// objectTypesIn returns the object types a type is built from, looking through
// the parameters of a list or a map.
func objectTypesIn(t *types.Type) []string {
	var names []string
	if t.Kind() == types.StructKind {
		names = append(names, t.TypeName())
	}
	for _, param := range t.Parameters() {
		names = append(names, objectTypesIn(param)...)
	}
	return names
}
