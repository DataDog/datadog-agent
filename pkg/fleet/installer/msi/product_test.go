// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package msi

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/registry"
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
			findProduct := func(name string) (*Product, error) {
				for _, installed := range tt.installed {
					if installed == name {
						return &Product{Code: "{test-product}"}, nil
					}
				}
				return nil, errProductNotFound
			}
			err := checkAgentFlavor(tt.fipsMode, findProduct)
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCheckAgentFlavorPropagatesLookupError(t *testing.T) {
	lookupErr := errors.New("uninstall registry query failed")
	err := checkAgentFlavor(false, func(string) (*Product, error) {
		return nil, lookupErr
	})
	require.ErrorIs(t, err, lookupErr)
}

func TestCheckAgentFlavorUninstallRegistry(t *testing.T) {
	for _, tt := range []struct {
		name      string
		fipsMode  bool
		installed []string
		wantError string
	}{
		{name: "clean host"},
		{name: "same flavor", installed: []string{"Datadog Agent"}},
		{name: "standard over fips", installed: []string{"Datadog FIPS Agent"}, wantError: "use the FIPS installer"},
		{name: "fips over standard", fipsMode: true, installed: []string{"Datadog Agent"}, wantError: "use the standard installer"},
		{name: "unreadable entry", installed: []string{"unreadable"}},
		{name: "unreadable entry before opposite flavor", installed: []string{"unreadable", "Datadog FIPS Agent"}, wantError: "use the FIPS installer"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rootPath := fmt.Sprintf(`Software\DatadogAgentFlavorTest-%d`, time.Now().UnixNano())
			root, _, err := registry.CreateKey(registry.CURRENT_USER, rootPath, registry.ALL_ACCESS)
			require.NoError(t, err)
			var createdKeys []string
			t.Cleanup(func() {
				for _, path := range createdKeys {
					require.NoError(t, registry.DeleteKey(registry.CURRENT_USER, path))
				}
				require.NoError(t, registry.DeleteKey(registry.CURRENT_USER, rootPath))
			})
			require.NoError(t, root.Close())

			for i, name := range tt.installed {
				path := fmt.Sprintf(`%s\product-%d`, rootPath, i)
				key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
				require.NoError(t, err)
				createdKeys = append(createdKeys, path)
				if name == "unreadable" {
					require.NoError(t, key.SetDWordValue("DisplayName", 1))
				} else {
					require.NoError(t, key.SetStringValue("DisplayName", name))
				}
				require.NoError(t, key.Close())
			}

			findProduct := func(name string) (*Product, error) {
				products, err := findUninstallProductCodes(registry.CURRENT_USER, rootPath, name)
				if err != nil {
					return nil, err
				}
				return products[0], nil
			}
			err = checkAgentFlavor(tt.fipsMode, findProduct)
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCheckAgentFlavorUninstallRegistryUnavailable(t *testing.T) {
	findProduct := func(name string) (*Product, error) {
		products, err := findUninstallProductCodes(registry.CURRENT_USER, `Software\DatadogAgentFlavorTest-Missing`, name)
		if err != nil {
			return nil, err
		}
		return products[0], nil
	}
	err := checkAgentFlavor(false, findProduct)
	require.ErrorContains(t, err, "could not check installed Agent flavor")
	require.ErrorIs(t, err, registry.ErrNotExist)
	require.NotErrorIs(t, err, errProductNotFound)
}
