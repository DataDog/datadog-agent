// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package gnmi implements the NDM gNMI core check.
package gnmi

import gnmi "github.com/openconfig/gnmi/proto/gnmi"

// DefaultEncoding is the gNMI subscription encoding requested from devices.
const DefaultEncoding = gnmi.Encoding_PROTO
