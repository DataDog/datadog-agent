// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package msi

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

var errProductNotFound = errors.New("no products found")

const uninstallRegistryPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`

// CheckAgentFlavor rejects an installer that targets the opposite installed Agent flavor.
func CheckAgentFlavor(fipsMode bool) error {
	findUninstallProducts := func(name string) ([]*Product, error) {
		return findUninstallRegistryProductCodes(registry.LOCAL_MACHINE, uninstallRegistryPath, name, registry.WOW64_64KEY, registry.WOW64_32KEY)
	}
	return checkAgentFlavor(fipsMode, findUninstallProducts, FindMsiProductCodesByName, isMsiProductInstalled)
}

func checkAgentFlavor(fipsMode bool, findUninstallRegistryProducts func(string) ([]*Product, error), findMsiProducts func(string) ([]Product, error), isInstalled func(string) (bool, error)) error {
	installedProduct := AgentProductName(!fipsMode)
	registryProducts, err := findUninstallRegistryProducts(installedProduct)
	if err != nil && !errors.Is(err, errProductNotFound) {
		return fmt.Errorf("could not check installed Agent flavor: %w", err)
	}
	msiProducts, _ := findMsiProducts(installedProduct) // An unrelated MSI enumeration failure must not block setup.

	seen := make(map[string]bool)
	checkProduct := func(code string) (bool, error) {
		if seen[code] {
			return false, nil
		}
		seen[code] = true
		return isInstalled(code)
	}
	for _, product := range registryProducts {
		installed, err := checkProduct(product.Code)
		if err != nil {
			return fmt.Errorf("could not check installed Agent flavor: %w", err)
		}
		if installed {
			return agentFlavorMismatchError(fipsMode, installedProduct)
		}
	}
	for _, product := range msiProducts {
		installed, err := checkProduct(product.Code)
		if err != nil {
			return fmt.Errorf("could not check installed Agent flavor: %w", err)
		}
		if installed {
			return agentFlavorMismatchError(fipsMode, installedProduct)
		}
	}
	return nil
}

func agentFlavorMismatchError(fipsMode bool, installedProduct string) error {
	matchingInstaller := "FIPS"
	if fipsMode {
		matchingInstaller = "standard"
	}
	return fmt.Errorf("cannot install %s while %s is installed; use the %s installer to manage the existing Agent", AgentProductName(fipsMode), installedProduct, matchingInstaller)
}

// isMsiProductInstalled queries the candidate product code directly, without scanning unrelated MSI products.
func isMsiProductInstalled(code string) (bool, error) {
	guid, err := windows.GUIDFromString(code)
	if err != nil {
		// Non-MSI uninstall entries can have arbitrary key names.
		return false, nil
	}
	code = guid.String()
	codePtr, err := windows.UTF16PtrFromString(code)
	if err != nil {
		return false, err
	}
	state, _, _ := windows.NewLazySystemDLL("msi.dll").NewProc("MsiQueryProductStateW").Call(uintptr(unsafe.Pointer(codePtr)))
	switch int32(state) {
	case 5: // INSTALLSTATE_DEFAULT: installed
		return true, nil
	case -1, 1: // INSTALLSTATE_UNKNOWN or INSTALLSTATE_ADVERTISED: not installed
		return false, nil
	default:
		return false, fmt.Errorf("query MSI product %s: unexpected state %d", code, int32(state))
	}
}

// readMSIProductName reads ProductName without installing the package.
func readMSIProductName(filename string) (string, error) {
	filenamePtr, err := windows.UTF16PtrFromString(filename)
	if err != nil {
		return "", err
	}
	// MSI handles must be closed on the thread that created them.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	dll := windows.NewLazySystemDLL("msi.dll")
	closeHandle := dll.NewProc("MsiCloseHandle")
	var database, view, record uint32
	ret, _, _ := dll.NewProc("MsiOpenDatabaseW").Call(
		uintptr(unsafe.Pointer(filenamePtr)), 0, uintptr(unsafe.Pointer(&database)), // MSIDBOPEN_READONLY
	)
	if ret != 0 {
		return "", fmt.Errorf("open MSI database: %w", windows.Errno(ret))
	}
	defer func() { _, _, _ = closeHandle.Call(uintptr(database)) }()

	query := windows.StringToUTF16Ptr("SELECT `Value` FROM `Property` WHERE `Property` = 'ProductName'")
	ret, _, _ = dll.NewProc("MsiDatabaseOpenViewW").Call(uintptr(database), uintptr(unsafe.Pointer(query)), uintptr(unsafe.Pointer(&view)))
	if ret != 0 {
		return "", fmt.Errorf("open MSI product query: %w", windows.Errno(ret))
	}
	defer func() { _, _, _ = closeHandle.Call(uintptr(view)) }()
	ret, _, _ = dll.NewProc("MsiViewExecute").Call(uintptr(view), 0)
	if ret != 0 {
		return "", fmt.Errorf("execute MSI product query: %w", windows.Errno(ret))
	}
	ret, _, _ = dll.NewProc("MsiViewFetch").Call(uintptr(view), uintptr(unsafe.Pointer(&record)))
	if ret != 0 {
		return "", fmt.Errorf("fetch MSI product: %w", windows.Errno(ret))
	}
	defer func() { _, _, _ = closeHandle.Call(uintptr(record)) }()

	var product [256]uint16
	size := uint32(len(product))
	ret, _, _ = dll.NewProc("MsiRecordGetStringW").Call(uintptr(record), 1, uintptr(unsafe.Pointer(&product[0])), uintptr(unsafe.Pointer(&size)))
	if ret != 0 {
		return "", fmt.Errorf("read MSI ProductName: %w", windows.Errno(ret))
	}
	return windows.UTF16ToString(product[:]), nil
}

// Product represents software found in the uninstall registry or Windows Installer.
type Product struct {
	// Code is the software product code
	Code string
	// UninstallString is the string that can be executed to uninstall the software. May be empty.
	UninstallString string
	// Features is a list of features installed by the product.
	Features []string
}

// FindProductCodeFromUninstallRegistry finds the first product with the specified display name in the uninstall registry.
func FindProductCodeFromUninstallRegistry(productName string) (*Product, error) {
	products, err := FindUninstallRegistryProductCodes(productName)
	if err != nil {
		return nil, err
	}
	return products[0], nil
}

// FindUninstallRegistryProductCodes looks for the productName in the machine uninstall registry.
func FindUninstallRegistryProductCodes(productName string) ([]*Product, error) {
	return findUninstallRegistryProductCodes(registry.LOCAL_MACHINE, uninstallRegistryPath, productName, 0)
}

// findUninstallRegistryProductCodes searches the requested registry views. A missing optional
// 32-bit uninstall location does not hide results from the 64-bit view.
func findUninstallRegistryProductCodes(root registry.Key, rootPath, productName string, views ...uint32) ([]*Product, error) {
	var products []*Product
	for _, view := range views {
		reg, err := registry.OpenKey(root, rootPath, registry.ENUMERATE_SUB_KEYS|view)
		if view == registry.WOW64_32KEY && errors.Is(err, registry.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		keys, err := reg.ReadSubKeyNames(0)
		_ = reg.Close()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			product, err := processKey(root, rootPath, key, productName, view)
			if err == nil && product != nil {
				products = append(products, product)
			}
		}
	}
	if len(products) == 0 {
		return nil, fmt.Errorf("%w with name: %s", errProductNotFound, productName)
	}
	return products, nil
}

// IsProductInstalled returns true if the given productName is installed
func IsProductInstalled(productName string) bool {
	product, err := FindProductCodeFromUninstallRegistry(productName)
	if err != nil {
		return false
	}
	return product != nil
}

// RemoveProduct uses the registry to try and find a product and use msiexec to remove it.
// It is different from msiexec in that it uses the registry and not the stable/experiment path on disk to
// uninstall the product.
// This is needed because in certain circumstances the installer database stored in the stable/experiment paths does not
// reflect the installed version, and using those installers can lead to undefined behavior (either failure to uninstall,
// or weird bugs from uninstalling a product with an installer from a different version).
func RemoveProduct(ctx context.Context, productName string, opts ...MsiexecOption) error {
	options := []MsiexecOption{
		Uninstall(),
		WithProduct(productName),
	}
	options = append(options, opts...)
	cmd, err := Cmd(options...)
	if err != nil {
		return fmt.Errorf("failed to remove product: %w", err)
	}
	err = cmd.Run(ctx)
	if err != nil {
		err = fmt.Errorf("failed to remove product: %w", err)
		var msiErr *MsiexecError
		if errors.As(err, &msiErr) {
			err = fmt.Errorf("%w\n%s", err, msiErr.ProcessedLog)
		}
		return err
	}
	return nil
}

func processKey(root registry.Key, rootPath, key, name string, view uint32) (*Product, error) {
	subkey, err := registry.OpenKey(root, rootPath+"\\"+key, registry.QUERY_VALUE|view)
	if err != nil {
		return nil, err
	}
	defer subkey.Close()

	displayName, _, err := subkey.GetStringValue("DisplayName")
	if err == nil && displayName == name {
		product := &Product{}
		product.UninstallString, _, _ = subkey.GetStringValue("UninstallString")
		product.Code = key
		return product, nil
	}

	return nil, nil
}

// FindMsiProductCodesByName enumerates Windows Installer products and matches their ProductName.
// An unreadable product name is skipped so other products can still be found.
func FindMsiProductCodesByName(productName string) ([]Product, error) {
	return findMsiProductCodesByName(productName, winutil.EnumerateMsiProducts, winutil.GetMsiProductInfo)
}

func findMsiProductCodesByName(productName string, enumerate func(uint32, func([]uint16, uint32, string) error) error, getProductInfo func(string, []uint16) (string, error)) ([]Product, error) {
	var products []Product

	err := enumerate(winutil.MSIINSTALLCONTEXT_MACHINE, func(productCode []uint16, _ uint32, _ string) error {
		displayName, err := getProductInfo("ProductName", productCode)
		if err != nil {
			return nil // One unreadable product must not hide later matches.
		}
		if displayName == productName {
			features, err := GetProductFeatures(productCode)
			if err != nil {
				features = append(features, fmt.Sprintf("error getting features: %v", err))
			}
			product := Product{
				Code:     windows.UTF16ToString(productCode[:]),
				Features: features,
			}
			products = append(products, product)
		}
		return nil
	})
	if err != nil {
		return products, err // Keep matches found before enumeration failed.
	}

	if len(products) == 0 {
		return nil, errProductNotFound
	}

	return products, nil
}

// GetProductFeatures enumberates all features for a given product code and returns them as a list of strings.
func GetProductFeatures(productCode []uint16) ([]string, error) {
	var features []string
	var index uint32
	bufferSize := uint32(windows.MAX_PATH)

	for {
		featureBuf := make([]uint16, bufferSize)
		parentBuf := make([]uint16, bufferSize)

		ret := winutil.MsiEnumFeatures(&productCode[0], index, &featureBuf[0], &parentBuf[0])

		if errors.Is(ret, windows.ERROR_NO_MORE_ITEMS) {
			break
		}
		if errors.Is(ret, windows.ERROR_MORE_DATA) {
			bufferSize++
			continue
		}
		if !errors.Is(ret, windows.ERROR_SUCCESS) {
			return nil, fmt.Errorf("error enumerating features: %w", ret)
		}

		// Just use UTF16ToString which will find the null terminator automatically
		// This ignores the potentially corrupted bufLen value
		feature := windows.UTF16ToString(featureBuf)
		if feature != "" { // Only add non-empty features
			features = append(features, feature)
		}
		index++
	}

	return features, nil
}
