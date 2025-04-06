// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

var errNonStaticPacketFilterField = errors.New("packet filter fields only support matching a single static")

func errorNonStaticPacketFilterField(a eval.Evaluator, b eval.Evaluator) error {
	var field string
	if a.IsStatic() {
		field = b.GetField()
	} else if b.IsStatic() {
		field = a.GetField()
	} else {
		return errNonStaticPacketFilterField
	}
	return fmt.Errorf("field `%s` only supports matching a single static value", field)
}
