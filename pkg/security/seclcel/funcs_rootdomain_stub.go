// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !(linux && seclmax) && !(linux && test)

package seclcel

import (
	"fmt"

	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// celRootDomain is unavailable outside the builds that carry SECL's own root
// domain resolution, so the helper reports that rather than answering wrongly.
func celRootDomain(ref.Val) ref.Val {
	return types.WrapErr(fmt.Errorf("%w: root_domain needs a Linux seclmax build", errUnsupportedValue))
}
