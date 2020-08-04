// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package dot

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
)

func TestDotWriterParenthesis(t *testing.T) {
	rule, err := ast.ParseRule(`(1) == (1)`)
	if err != nil {
		t.Error(err)
	}

	dotMarshaller := NewMarshaler(os.Stdout)

	if err := dotMarshaller.MarshalRule(rule); err != nil {
		t.Error(err)
	}
}

func TestDotWriterInArray(t *testing.T) {
	rule, err := ast.ParseRule(`3 in [1, 2, 3]`)
	if err != nil {
		t.Error(err)
	}

	dotMarshaller := NewMarshaler(os.Stdout)

	if err := dotMarshaller.MarshalRule(rule); err != nil {
		t.Error(err)
	}
}
