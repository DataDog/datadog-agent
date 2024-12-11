// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package ndmtmp implements the "ndmtmp" bundle, which exposes the default
// sender.Sender and the event platform forwarder. This is a temporary module
// intended for ndm internal use until these pieces are properly componentized.

//go:build test

package ndmtmp

import (
	"github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder/forwarderimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: ndm-core

// MockBundle defines the fx options for mock versions of everything in this bundle.
func MockBundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		forwarderimpl.MockModule())
}
