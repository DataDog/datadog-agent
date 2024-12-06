// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"
)

type version int

const (
	instrumentationVersionInvalid version = iota
	instrumentationV1
	instrumentationV2
)

func instrumentationVersion(v string) (version, error) {
	switch v {
	case "v1":
		return instrumentationV1, nil
	case "v2":
		return instrumentationV2, nil
	default:
		return instrumentationVersionInvalid, fmt.Errorf("invalid version: %v", v)
	}
}

func (v version) usesInjector() bool {
	switch v {
	case instrumentationV1:
		return false
	case instrumentationV2:
		return true
	default:
		// N.B. version is validated on construction.
		// So this code is _generally_ unreachable within the webhook.
		//
		// Another would have been to pick a default value, but then we might have
		// a weird code path somewhere.
		panic(fmt.Errorf("invalid instrumentation version %v", v))
	}
}
