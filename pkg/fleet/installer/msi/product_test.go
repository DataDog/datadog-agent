// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package msi

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckAgentFlavor(t *testing.T) {
	for _, tt := range []struct {
		name      string
		fipsMode  bool
		installed []string
		wantError string
	}{
		{name: "clean host standard"},
		{name: "clean host fips", fipsMode: true},
		{name: "standard over standard", installed: []string{"Datadog Agent"}},
		{name: "fips over fips", fipsMode: true, installed: []string{"Datadog FIPS Agent"}},
		{name: "standard over fips", installed: []string{"Datadog FIPS Agent"}, wantError: "use the FIPS installer"},
		{name: "fips over standard", fipsMode: true, installed: []string{"Datadog Agent"}, wantError: "use the standard installer"},
		{name: "both installed", installed: []string{"Datadog Agent", "Datadog FIPS Agent"}, wantError: "use the FIPS installer"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			findProducts := func(name string) ([]Product, error) {
				for _, installed := range tt.installed {
					if installed == name {
						return []Product{{Code: "{test-product}"}}, nil
					}
				}
				return nil, errProductNotFound
			}
			err := checkAgentFlavor(tt.fipsMode, findProducts)
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCheckAgentFlavorPropagatesLookupError(t *testing.T) {
	lookupErr := errors.New("Windows Installer query failed")
	err := checkAgentFlavor(false, func(string) ([]Product, error) {
		return nil, lookupErr
	})
	require.ErrorIs(t, err, lookupErr)
}
