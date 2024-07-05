// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package newstyleimpl

import (
	newstyle "github.com/DataDog/datadog-agent/comp/newstyle/def"
)

type Requires struct{}

type Provides struct {
	Comp newstyle.Component
}

type implementation struct{}

func NewComponent(reqs Requires) Provides {
	return Provides{
		Comp: &implementation{},
	}
}
