// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trivy

import (
	cyclonedxgo "github.com/CycloneDX/cyclonedx-go"
)

// Report interface
type Report interface {
	ToCycloneDX() (*cyclonedxgo.BOM, error)
}

// CacheCleaner interface
type CacheCleaner interface {
	Clean() error
	setKeysForEntity(entity string, cachedKeys []string)
}
