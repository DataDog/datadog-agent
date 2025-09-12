// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
)

// MetadataProviders helps unmarshalling `metadata_providers` config param
type MetadataProviders struct {
	Name          string        `mapstructure:"name"`
	Interval      time.Duration `mapstructure:"interval"`
	EarlyInterval time.Duration `mapstructure:"early_interval"`
}

// GetMetadataProviders returns the "metadata_providers" set in the configuration
func GetMetadataProviders(c pkgconfigmodel.Reader) ([]MetadataProviders, error) {
	var mp []MetadataProviders
	return mp, structure.UnmarshalKey(c, "metadata_providers", &mp)
}
