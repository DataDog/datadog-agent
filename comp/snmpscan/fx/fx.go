// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the snmpscan component
package fx

import (
	snmpscan "github.com/DataDog/datadog-agent/comp/snmpscan/def"
	snmpscanimpl "github.com/DataDog/datadog-agent/comp/snmpscan/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			snmpscanimpl.NewComponent,
		),
		fxutil.ProvideOptional[snmpscan.Component](),
	)
}
