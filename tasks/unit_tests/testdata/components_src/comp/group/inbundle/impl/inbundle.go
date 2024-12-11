// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inbundleimpl

import (
	inbundle "github.com/DataDog/datadog-agent/comp/group/inbundle/def"
)

type Requires struct{}

type Provides struct {
	Comp inbundle.Component
}

type implementation struct{}

func NewComponent(reqs Requires) Provides {
	return Provides{
		Comp: &implementation{},
	}
}
