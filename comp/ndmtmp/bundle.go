// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package ndmtmp implements the "ndmtmp" bundle, which exposes the default
// sender.Sender and the event platform forwarder. This is a temporary module
// intended for ndm internal use until these pieces are properly componentized.
package ndmtmp

import (
	"github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder/forwarderimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: ndm-core

// TODO: (components) Delete this module when the sender and event platform forwarder are fully componentized.

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		forwarderimpl.Module())
}
