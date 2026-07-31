// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// The build constraint mirrors compiler/eval/strings_linux.go, which is where the
// root domain resolution lives: SECL only offers it on Linux and only in the
// full featured build, so the CEL branch offers it in exactly the same places.

//go:build (linux && seclmax) || (linux && test)

package seclcel

import (
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// celRootDomain extracts the effective root domain of a host name, backing
// SECL's `root_domain` suffix.
func celRootDomain(val ref.Val) ref.Val {
	domain, ok := val.(types.String)
	if !ok {
		return types.MaybeNoSuchOverloadErr(val)
	}
	return types.String(eval.EffectiveTLDPlusOneWithFallback(string(domain)))
}
