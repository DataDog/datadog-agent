// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types implements the types used by the Tagger for Origin Detection.
package types

import "github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"

// OriginInfo contains the Origin Detection information.
type OriginInfo struct {
	ContainerIDFromSocket string                        // ContainerIDFromSocket is the origin resolved using Unix Domain Socket.
	LocalData             origindetection.LocalData     // LocalData is the local data list.
	ExternalData          origindetection.ExternalData  // ExternalData is the external data list.
	Cardinality           string                        // Cardinality is the cardinality of the resolved origin.
	ProductOrigin         origindetection.ProductOrigin // ProductOrigin is the product that sent the origin information.
}
