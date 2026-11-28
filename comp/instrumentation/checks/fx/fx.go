// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package fx wires the checks instrumentation component.
package fx

import (
	checksimpl "github.com/DataDog/datadog-agent/comp/instrumentation/checks/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module provides the checks instrumentation component.
func Module() fxutil.Module {
	return fxutil.Component(fxutil.ProvideComponentConstructor(checksimpl.NewComponent))
}
