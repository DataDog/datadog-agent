// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package msi

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
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
			findProducts := func(name string) ([]*Product, error) {
				for _, installed := range tt.installed {
					if installed == name {
						return []*Product{{Code: "{test-product}"}}, nil
					}
				}
				return nil, errProductNotFound
			}
			err := checkAgentFlavor(tt.fipsMode, findProducts, func(string) (bool, error) { return true, nil })
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
	err := checkAgentFlavor(false, func(string) ([]*Product, error) {
		return nil, lookupErr
	}, func(string) (bool, error) { return false, nil })
	require.ErrorIs(t, err, lookupErr)
}

func TestCheckAgentFlavorConfirmsEachProductCode(t *testing.T) {
	lookup := func(string) ([]*Product, error) {
		return []*Product{{Code: "stale"}, {Code: "installed"}}, nil
	}
	t.Run("stale entry before installed product", func(t *testing.T) {
		var checked []string
		err := checkAgentFlavor(false, lookup, func(code string) (bool, error) {
			checked = append(checked, code)
			return code == "installed", nil
		})
		require.ErrorContains(t, err, "use the FIPS installer")
		require.Equal(t, []string{"stale", "installed"}, checked)
	})
	t.Run("only stale entries", func(t *testing.T) {
		err := checkAgentFlavor(false, lookup, func(string) (bool, error) { return false, nil })
		require.NoError(t, err)
	})
	t.Run("MSI query error", func(t *testing.T) {
		queryErr := errors.New("MSI query failed")
		err := checkAgentFlavor(false, lookup, func(string) (bool, error) { return false, queryErr })
		require.ErrorIs(t, err, queryErr)
	})
}

func TestFindAgentFlavorProductsInViews(t *testing.T) {
	var views []uint32
	products, err := findAgentFlavorProductsInViews("Datadog FIPS Agent", func(_ string, view uint32) ([]*Product, error) {
		views = append(views, view)
		if view == registry.WOW64_32KEY {
			return []*Product{{Code: "32-bit-product"}}, nil
		}
		return nil, errProductNotFound
	})
	require.NoError(t, err)
	require.Equal(t, []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY}, views)
	require.Equal(t, "32-bit-product", products[0].Code)

	_, err = findAgentFlavorProductsInViews("Datadog FIPS Agent", func(_ string, view uint32) ([]*Product, error) {
		if view == registry.WOW64_32KEY {
			return nil, registry.ErrNotExist // The 32-bit uninstall location need not exist.
		}
		return nil, errProductNotFound
	})
	require.ErrorIs(t, err, errProductNotFound)

	_, err = findAgentFlavorProductsInViews("Datadog FIPS Agent", func(_ string, view uint32) ([]*Product, error) {
		if view == registry.WOW64_32KEY {
			return nil, windows.ERROR_ACCESS_DENIED
		}
		return nil, errProductNotFound
	})
	require.ErrorIs(t, err, windows.ERROR_ACCESS_DENIED)

	_, err = findAgentFlavorProductsInViews("Datadog FIPS Agent", func(_ string, view uint32) ([]*Product, error) {
		if view == registry.WOW64_64KEY {
			return nil, registry.ErrNotExist
		}
		return nil, errProductNotFound
	})
	require.ErrorIs(t, err, registry.ErrNotExist)
}

func TestIsMsiProductInstalledUnknown(t *testing.T) {
	guid, err := windows.GenerateGUID()
	require.NoError(t, err)
	installed, err := isMsiProductInstalled(guid.String())
	require.NoError(t, err)
	require.False(t, installed)

	installed, err = isMsiProductInstalled("not an MSI product code")
	require.NoError(t, err)
	require.False(t, installed)
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

			findProducts := func(name string) ([]*Product, error) {
				return findUninstallProductCodes(registry.CURRENT_USER, rootPath, name, 0)
			}
			err = checkAgentFlavor(tt.fipsMode, findProducts, func(string) (bool, error) { return true, nil })
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Set DD_TEST_INSTALLED_AGENT_FLAVOR on a Windows host with an installed MSI to exercise the real check.
func TestCheckAgentFlavorInstalledAgent(t *testing.T) {
	flavor := os.Getenv("DD_TEST_INSTALLED_AGENT_FLAVOR")
	if flavor == "" {
		t.Skip("requires an installed Agent MSI")
	}
	require.Contains(t, []string{"standard", "FIPS"}, flavor)
	fipsMode := flavor == "FIPS"
	require.NoError(t, CheckAgentFlavor(fipsMode))
	require.ErrorContains(t, CheckAgentFlavor(!fipsMode), "use the "+flavor+" installer")
}

func TestCheckAgentFlavorStaleUninstallEntry(t *testing.T) {
	guid, err := windows.GenerateGUID()
	require.NoError(t, err)
	rootPath := fmt.Sprintf(`Software\DatadogAgentFlavorTest-%d`, time.Now().UnixNano())
	keyPath := rootPath + `\` + guid.String()
	key, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.ALL_ACCESS)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, registry.DeleteKey(registry.CURRENT_USER, keyPath))
		require.NoError(t, registry.DeleteKey(registry.CURRENT_USER, rootPath))
	})
	require.NoError(t, key.SetStringValue("DisplayName", "Datadog FIPS Agent"))
	require.NoError(t, key.Close())

	findProducts := func(name string) ([]*Product, error) {
		return findUninstallProductCodes(registry.CURRENT_USER, rootPath, name, 0)
	}
	require.NoError(t, checkAgentFlavor(false, findProducts, isMsiProductInstalled))
}

func TestCheckAgentFlavorUninstallRegistryUnavailable(t *testing.T) {
	findProducts := func(name string) ([]*Product, error) {
		return findUninstallProductCodes(registry.CURRENT_USER, `Software\DatadogAgentFlavorTest-Missing`, name, 0)
	}
	err := checkAgentFlavor(false, findProducts, func(string) (bool, error) { return true, nil })
	require.ErrorContains(t, err, "could not check installed Agent flavor")
	require.ErrorIs(t, err, registry.ErrNotExist)
	require.NotErrorIs(t, err, errProductNotFound)
}
