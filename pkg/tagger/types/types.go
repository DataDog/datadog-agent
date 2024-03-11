// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types implements the types used by the Tagger for Origin Detection.
package types

// ProductOrigin is the origin of the product that sent the entity.
type ProductOrigin string

const (
	// ProductOriginDogStatsD is the ProductOrigin for DogStatsD.
	ProductOriginDogStatsD ProductOrigin = "dogstatsd"
	// ProductOriginAPM is the ProductOrigin for APM.
	ProductOriginAPM ProductOrigin = "apm"
)

// OriginInfo contains the Origin Detection information.
type OriginInfo struct {
	FromUDS       string
	FromTag       string
	FromMsg       string
	Cardinality   string
	ProductOrigin ProductOrigin
	OptOutEnabled *bool
}
