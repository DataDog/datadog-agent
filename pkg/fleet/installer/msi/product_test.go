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

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

func noMsiProducts(string) ([]Product, error) { return nil, errProductNotFound }

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
			err := checkAgentFlavor(tt.fipsMode, findProducts, noMsiProducts, func(string) (bool, error) { return true, nil })
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
	}, noMsiProducts, func(string) (bool, error) { return false, nil })
	require.ErrorIs(t, err, lookupErr)
}

func TestCheckAgentFlavorConfirmsEachProductCode(t *testing.T) {
	lookup := func(string) ([]*Product, error) {
		return []*Product{{Code: "stale"}, {Code: "installed"}}, nil
	}
	t.Run("stale entry before installed product", func(t *testing.T) {
		var checked []string
		err := checkAgentFlavor(false, lookup, noMsiProducts, func(code string) (bool, error) {
			checked = append(checked, code)
			return code == "installed", nil
		})
		require.ErrorContains(t, err, "use the FIPS installer")
		require.Equal(t, []string{"stale", "installed"}, checked)
	})
	t.Run("only stale entries", func(t *testing.T) {
		err := checkAgentFlavor(false, lookup, noMsiProducts, func(string) (bool, error) { return false, nil })
		require.NoError(t, err)
	})
	t.Run("MSI query error", func(t *testing.T) {
		queryErr := errors.New("MSI query failed")
		err := checkAgentFlavor(false, lookup, noMsiProducts, func(string) (bool, error) { return false, queryErr })
		require.ErrorIs(t, err, queryErr)
	})
}

func TestCheckAgentFlavorCombinesProductCodeSources(t *testing.T) {
	lookupRegistry := func(string) ([]*Product, error) { return nil, errProductNotFound }
	lookupMsi := func(string) ([]Product, error) { return []Product{{Code: "msi-only"}}, nil }
	t.Run("MSI-only opposite flavor", func(t *testing.T) {
		err := checkAgentFlavor(false, lookupRegistry, lookupMsi, func(code string) (bool, error) {
			require.Equal(t, "msi-only", code)
			return true, nil
		})
		require.ErrorContains(t, err, "use the FIPS installer")
	})
	t.Run("MSI-only stale product", func(t *testing.T) {
		require.NoError(t, checkAgentFlavor(false, lookupRegistry, lookupMsi, func(string) (bool, error) { return false, nil }))
	})
	t.Run("duplicate code from both sources", func(t *testing.T) {
		checked := 0
		lookupRegistry := func(string) ([]*Product, error) { return []*Product{{Code: "same-code"}}, nil }
		lookupMsi := func(string) ([]Product, error) { return []Product{{Code: "same-code"}}, nil }
		err := checkAgentFlavor(false, lookupRegistry, lookupMsi, func(string) (bool, error) {
			checked++
			return false, nil
		})
		require.NoError(t, err)
		require.Equal(t, 1, checked)
	})
	t.Run("MSI enumeration failure does not block setup", func(t *testing.T) {
		enumErr := errors.New("unrelated MSI query failed")
		findMsi := func(string) ([]Product, error) { return nil, enumErr }
		require.NoError(t, checkAgentFlavor(false, lookupRegistry, findMsi, func(string) (bool, error) { return false, nil }))
	})
	t.Run("MSI match before enumeration failure still rejects", func(t *testing.T) {
		enumErr := errors.New("later MSI query failed")
		findMsi := func(string) ([]Product, error) { return []Product{{Code: "msi-only"}}, enumErr }
		err := checkAgentFlavor(false, lookupRegistry, findMsi, func(string) (bool, error) { return true, nil })
		require.ErrorContains(t, err, "use the FIPS installer")
	})
	t.Run("registry failure still blocks setup", func(t *testing.T) {
		registryErr := errors.New("uninstall registry unavailable")
		lookupRegistry := func(string) ([]*Product, error) { return nil, registryErr }
		err := checkAgentFlavor(false, lookupRegistry, lookupMsi, func(string) (bool, error) { return false, nil })
		require.ErrorIs(t, err, registryErr)
	})
}

func TestFindMsiProductCodesByNameSkipsUnreadableProducts(t *testing.T) {
	badCode := "{00000000-0000-0000-0000-000000000001}"
	goodCode := "{00000000-0000-0000-0000-000000000002}"
	enumerate := func(_ uint32, visit func([]uint16, uint32, string) error) error {
		for _, code := range []string{badCode, goodCode} {
			if err := visit(windows.StringToUTF16(code), winutil.MSIINSTALLCONTEXT_MACHINE, ""); err != nil {
				return err
			}
		}
		return nil
	}
	getProductInfo := func(_ string, code []uint16) (string, error) {
		if windows.UTF16ToString(code) == badCode {
			return "", errors.New("unreadable unrelated MSI product")
		}
		return "Datadog FIPS Agent", nil
	}
	products, err := findMsiProductCodesByName("Datadog FIPS Agent", enumerate, getProductInfo)
	require.NoError(t, err)
	require.Len(t, products, 1)
	require.Equal(t, goodCode, products[0].Code)

	enumErr := errors.New("enumeration failed after the matching product")
	enumeratePartial := func(_ uint32, visit func([]uint16, uint32, string) error) error {
		require.NoError(t, visit(windows.StringToUTF16(goodCode), winutil.MSIINSTALLCONTEXT_MACHINE, ""))
		return enumErr
	}
	products, err = findMsiProductCodesByName("Datadog FIPS Agent", enumeratePartial, getProductInfo)
	require.ErrorIs(t, err, enumErr)
	require.Len(t, products, 1)
	require.Equal(t, goodCode, products[0].Code)
}

func TestFindUninstallRegistryProductCodesInViews(t *testing.T) {
	rootPath := fmt.Sprintf(`Software\DatadogAgentFlavorTest-%d`, time.Now().UnixNano())
	keyPath := rootPath + `\product`
	key, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.ALL_ACCESS)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, registry.DeleteKey(registry.CURRENT_USER, keyPath))
		require.NoError(t, registry.DeleteKey(registry.CURRENT_USER, rootPath))
	})
	require.NoError(t, key.SetStringValue("DisplayName", "Datadog FIPS Agent"))
	require.NoError(t, key.Close())

	// HKCU's uninstall path is shared between the 32- and 64-bit registry views.
	products, err := findUninstallRegistryProductCodes(registry.CURRENT_USER, rootPath, "Datadog FIPS Agent", registry.WOW64_64KEY, registry.WOW64_32KEY)
	require.NoError(t, err)
	require.Len(t, products, 2)
	require.Equal(t, "product", products[0].Code)
	require.Equal(t, "product", products[1].Code)
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
				return findUninstallRegistryProductCodes(registry.CURRENT_USER, rootPath, name, 0)
			}
			err = checkAgentFlavor(tt.fipsMode, findProducts, noMsiProducts, func(string) (bool, error) { return true, nil })
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
		return findUninstallRegistryProductCodes(registry.CURRENT_USER, rootPath, name, 0)
	}
	require.NoError(t, checkAgentFlavor(false, findProducts, noMsiProducts, isMsiProductInstalled))
}

func TestCheckAgentFlavorUninstallRegistryUnavailable(t *testing.T) {
	findProducts := func(name string) ([]*Product, error) {
		return findUninstallRegistryProductCodes(registry.CURRENT_USER, `Software\DatadogAgentFlavorTest-Missing`, name, 0)
	}
	err := checkAgentFlavor(false, findProducts, noMsiProducts, func(string) (bool, error) { return true, nil })
	require.ErrorContains(t, err, "could not check installed Agent flavor")
	require.ErrorIs(t, err, registry.ErrNotExist)
	require.NotErrorIs(t, err, errProductNotFound)
}
