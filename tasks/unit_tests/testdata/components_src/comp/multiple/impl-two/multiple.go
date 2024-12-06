// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package multipleimpl

import (
	multiple "github.com/DataDog/datadog-agent/comp/multiple/def"
)

type Requires struct{}

type Provides struct {
	Comp multiple.Component
}

type implementation2 struct{}

func NewComponent(reqs Requires) Provides {
	return Provides{
		Comp: &implementation2{},
	}
}
