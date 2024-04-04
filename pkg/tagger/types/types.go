// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types implements the types used by the Tagger for Origin Detection.
package types

// ProductOrigin is the origin of the product that sent the entity.
type ProductOrigin int

const (
	// ProductOriginDogStatsD is the ProductOrigin for DogStatsD.
	ProductOriginDogStatsD ProductOrigin = iota
	// ProductOriginAPM is the ProductOrigin for APM.
	ProductOriginAPM ProductOrigin = iota
)

// OriginInfo contains the Origin Detection information.
type OriginInfo struct {
	FromUDS       string        // FromUDS is the origin resolved using Unix Domain Socket.
	FromTag       string        // FromTag is the origin resolved from tags.
	FromMsg       string        // FromMsg is the origin resolved from the message.
	Cardinality   string        // Cardinality is the cardinality of the resolved origin.
	ProductOrigin ProductOrigin // ProductOrigin is the product that sent the origin information.
}
