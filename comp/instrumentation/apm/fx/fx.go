// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package fx wires the apm instrumentation component.
package fx

import (
	apmimpl "github.com/DataDog/datadog-agent/comp/instrumentation/apm/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module provides the apm instrumentation component.
func Module() fxutil.Module {
	return fxutil.Component(fxutil.ProvideComponentConstructor(apmimpl.NewComponent))
}
