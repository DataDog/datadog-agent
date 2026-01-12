// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package connfilter

import (
	"errors"
	"testing"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
)

func getConnFilter(t *testing.T, configString string, ddSite string, monitorIPWithoutDomain bool) (*ConnFilter, error) {
	var configs []Config

	cfg := configComponent.NewMockFromYAML(t, configString)
	err := structure.UnmarshalKey(cfg, "filters", &configs)
	if err != nil {
		return nil, err
	}
	connFilter, errs := NewConnFilter(configs, ddSite, monitorIPWithoutDomain)
	if len(errs) > 0 {
		err = errors.Join(errs...)
	}
	return connFilter, err
}
