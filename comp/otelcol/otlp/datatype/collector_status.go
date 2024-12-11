// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package datatype declares basic datatypes used by OTLP
package datatype

// CollectorStatus is the status struct for an OTLP pipeline's collector
type CollectorStatus struct {
	Status       string
	ErrorMessage string
}
