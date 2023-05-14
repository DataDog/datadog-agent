// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnosis

import "github.com/DataDog/datadog-agent/pkg/util/log"

// Catalog holds available diagnosis for detection and usage
type Catalog map[string]Diagnosis

// DefaultCatalog holds every compiled-in diagnosis
var DefaultCatalog = make(Catalog)

// Register a diagnosis that will be called on diagnose
func Register(name string, d Diagnosis) {
	if _, ok := DefaultCatalog[name]; ok {
		log.Warnf("Diagnosis %s already registered, overriding it", name)
	}
	DefaultCatalog[name] = d
}

// Diagnosis should return an error to report its health
type Diagnosis func() error
