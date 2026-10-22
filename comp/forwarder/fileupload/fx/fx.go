// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx provides the fx module for the file upload forwarder component.
package fx

import (
	fileuploadimpl "github.com/DataDog/datadog-agent/comp/forwarder/fileupload/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: cloud-network-monitoring

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			fileuploadimpl.NewComponent,
		),
	)
}
