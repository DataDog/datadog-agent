// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"go.yaml.in/yaml/v3"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/schema"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Product enablement lets users enable products (and SKUs, bundles of products) instead of configuring every
// setting those products need. Products are declared in the core schema:
//   - 'sku_definitions' maps a SKU to the products it bundles.
//   - 'product_dependencies' maps every product to the products it depends on (resolved recursively).
//
// Settings declare the value to use when a product is enabled with 'product_defaults' or
// 'product_platform_defaults'. Those values are written to the model.SourceProductEnablement layer, so any other
// source (file, env vars, ...) still overrides them.
//
// The schema definitions are validated by 'dda inv schema.lint', so the runtime only reports errors caused by the
// user configuration: unknown SKU or product, and products setting conflicting values.

// ErrProductEnablement is wrapped by every error caused by product enablement (unknown SKU or product, products
// setting conflicting values, ...). See SplitProductEnablementError.
var ErrProductEnablement = errors.New("product enablement")

// SchemaGetter returns the raw YAML of an embedded schema
type SchemaGetter func() ([]byte, error)

// Schemas used for product enablement (overridden by tests)
var (
	coreSchemaGetter        SchemaGetter = schema.GetCoreSchema
	systemProbeSchemaGetter SchemaGetter = schema.GetSystemProbeSchema
)

// loadAndProductError holds both the error loading a configuration and the product enablement error
type loadAndProductError struct {
	loadErr    error
	productErr error
}

func (e *loadAndProductError) Error() string {
	return e.loadErr.Error() + "\n" + e.productErr.Error()
}

// Unwrap lets errors.Is and errors.As match both errors
func (e *loadAndProductError) Unwrap() []error {
	return []error{e.loadErr, e.productErr}
}

// withProductEnablementError adds the product enablement error to the error returned when loading a configuration
func withProductEnablementError(loadErr, productErr error) error {
	if productErr == nil {
		return loadErr
	}
	if loadErr == nil {
		return productErr
	}
	return &loadAndProductError{loadErr: loadErr, productErr: productErr}
}

// SplitProductEnablementError splits an error returned by LoadDatadog or LoadSystemProbe between the error loading the
// configuration and the product enablement error, so callers can handle them differently.
func SplitProductEnablementError(err error) (loadErr error, productErr error) {
	var both *loadAndProductError
	if errors.As(err, &both) {
		return both.loadErr, both.productErr
	}
	if errors.Is(err, ErrProductEnablement) {
		return nil, err
	}
	return err, nil
}

// resolvedProducts holds the products enabled for the core configuration, see ResolvedProducts
var resolvedProducts struct {
	sync.RWMutex
	products []string
}

// ResolvedProducts returns the sorted list of products enabled for the core configuration by ApplyProductEnablement,
// including their dependencies.
func ResolvedProducts() []string {
	resolvedProducts.RLock()
	defer resolvedProducts.RUnlock()
	return slices.Clone(resolvedProducts.products)
}

func setResolvedProducts(products []string) {
	resolvedProducts.Lock()
	defer resolvedProducts.Unlock()
	resolvedProducts.products = products
}

// isTestBinary reports whether the code runs in a test binary (overridden by tests)
var isTestBinary = testing.Testing

// ApplyProductEnablement enables the products configured through the 'sku' and 'products' settings: the values the
// enabled products set in the core schema are written to the model.SourceProductEnablement layer of config.
//
// It's called by LoadDatadog once the file and environment variables are loaded (products set through fleet policies
// are not supported). Nothing is applied if an error is returned. The schema is only loaded if a product is enabled.
func ApplyProductEnablement(config pkgconfigmodel.ReaderWriter) error {
	setResolvedProducts(nil)

	resolved, coreRoot, err := resolveEnabledProducts(config)
	if err != nil || resolved == nil {
		return err
	}
	if err := applyProductDefaults(config, coreRoot, resolved); err != nil {
		return err
	}

	setResolvedProducts(resolved)
	log.Infof("Product enablement: sku=%s products=%v resolved=%v", config.GetString("sku"), config.GetStringSlice("products"), resolved)
	return nil
}

// ApplySystemProbeProductEnablement enables, in the system-probe configuration, the products configured through the
// 'sku' and 'products' settings of the core configuration: the values the enabled products set in the system-probe
// schema are written to the model.SourceProductEnablement layer of spConfig.
//
// It's called by LoadSystemProbe; the core configuration must be loaded first. Nothing is applied if an error is
// returned.
func ApplySystemProbeProductEnablement(spConfig pkgconfigmodel.ReaderWriter, coreConfig pkgconfigmodel.Config) error {
	// Tests routinely build the system-probe configuration without loading the core one (see
	// comp/core/sysprobeconfig/mock), the check would make all of them fail.
	if !coreConfig.IsLoaded() && !isTestBinary() {
		return fmt.Errorf("%w: the core configuration must be loaded before the system-probe configuration", ErrProductEnablement)
	}

	resolved, _, err := resolveEnabledProducts(coreConfig)
	if err != nil || resolved == nil {
		return err
	}
	systemProbeRoot, err := loadSchema(systemProbeSchemaGetter)
	if err != nil {
		return err
	}
	return applyProductDefaults(spConfig, systemProbeRoot, resolved)
}

// resolveEnabledProducts returns the products enabled by the 'sku' and 'products' settings of the core configuration,
// with their dependencies, and the decoded core schema. It returns nil products, without loading the schema, if no
// product is enabled.
func resolveEnabledProducts(coreConfig pkgconfigmodel.Reader) ([]string, map[string]interface{}, error) {
	sku := coreConfig.GetString("sku")
	products := coreConfig.GetStringSlice("products")
	if sku == "" && len(products) == 0 {
		return nil, nil, nil
	}

	coreRoot, err := loadSchema(coreSchemaGetter)
	if err != nil {
		return nil, nil, err
	}
	skuDefinitions, productDependencies := productDefinitions(coreRoot)

	resolved, err := resolveProducts(sku, products, skuDefinitions, productDependencies)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrProductEnablement, err)
	}
	return resolved, coreRoot, nil
}

// applyProductDefaults writes the values set by the enabled products in the schema to the
// model.SourceProductEnablement layer of config. Nothing is written if the products set conflicting values.
func applyProductDefaults(config pkgconfigmodel.ReaderWriter, schemaRoot map[string]interface{}, enabled []string) error {
	defaults, err := collectProductDefaults(schemaRoot, enabled)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrProductEnablement, err)
	}
	for key, value := range defaults {
		config.Set(key, value, pkgconfigmodel.SourceProductEnablement)
	}
	return nil
}

// loadSchema decodes a schema as a generic tree
func loadSchema(getSchema SchemaGetter) (map[string]interface{}, error) {
	content, err := getSchema()
	if err != nil {
		return nil, fmt.Errorf("%w: could not load the schema: %w", ErrProductEnablement, err)
	}
	var root map[string]interface{}
	if err := yaml.Unmarshal(content, &root); err != nil {
		return nil, fmt.Errorf("%w: could not load the schema: %w", ErrProductEnablement, err)
	}
	return root, nil
}

// productDefinitions returns the 'sku_definitions' and 'product_dependencies' maps of a schema. Their format is
// enforced by 'dda inv schema.lint', malformed entries are ignored.
func productDefinitions(schemaRoot map[string]interface{}) (skuDefinitions, productDependencies map[string][]string) {
	toMap := func(raw interface{}) map[string][]string {
		definitions := map[string][]string{}
		entries, _ := raw.(map[string]interface{})
		for name, rawList := range entries {
			list, _ := rawList.([]interface{})
			names := make([]string, 0, len(list))
			for _, item := range list {
				if name, ok := item.(string); ok {
					names = append(names, name)
				}
			}
			definitions[name] = names
		}
		return definitions
	}
	return toMap(schemaRoot["sku_definitions"]), toMap(schemaRoot["product_dependencies"])
}

// resolveProducts returns the sorted list of products enabled by a SKU and a list of products, including all their
// dependencies.
func resolveProducts(sku string, products []string, skuDefinitions, productDependencies map[string][]string) ([]string, error) {
	requested := slices.Clone(products)
	for _, product := range products {
		if _, isSKU := skuDefinitions[product]; isSKU {
			return nil, fmt.Errorf("'%s' is a SKU, it must be set with 'sku' instead of 'products'", product)
		}
		if _, found := productDependencies[product]; !found {
			return nil, fmt.Errorf("unknown product '%s'", product)
		}
	}
	if sku != "" {
		skuProducts, found := skuDefinitions[sku]
		if !found {
			return nil, fmt.Errorf("unknown SKU '%s'", sku)
		}
		requested = append(requested, skuProducts...)
	}

	enabled := map[string]struct{}{}
	for len(requested) > 0 {
		product := requested[len(requested)-1]
		requested = requested[:len(requested)-1]
		if _, seen := enabled[product]; seen {
			continue
		}
		enabled[product] = struct{}{}
		requested = append(requested, productDependencies[product]...)
	}

	resolved := make([]string, 0, len(enabled))
	for product := range enabled {
		resolved = append(resolved, product)
	}
	slices.Sort(resolved)
	return resolved, nil
}

// productValue is the value a product sets for a setting
type productValue struct {
	product string
	value   interface{}
}

// collectProductDefaults walks the schema and returns the value of every setting changed by the enabled products,
// indexed by setting name. Platform specific values are resolved for the current platform, like 'platform_default'.
//
// An error listing every setting is returned if the enabled products set different values for the same setting.
func collectProductDefaults(schemaRoot map[string]interface{}, enabled []string) (map[string]interface{}, error) {
	defaults := map[string]interface{}{}
	var conflicts []string

	var walk func(node map[string]interface{}, prefix string)
	walk = func(node map[string]interface{}, prefix string) {
		properties, _ := node["properties"].(map[string]interface{})
		names := make([]string, 0, len(properties))
		for name := range properties {
			names = append(names, name)
		}
		// Sort the settings so conflicts are always reported in the same order
		slices.Sort(names)

		for _, name := range names {
			child, ok := properties[name].(map[string]interface{})
			if !ok {
				continue
			}
			key := name
			if prefix != "" {
				key = prefix + "." + name
			}

			if child["node_type"] == "section" {
				walk(child, key)
				continue
			}

			values := productValues(child, enabled)
			if len(values) == 0 {
				continue
			}
			if !allEqual(values) {
				conflicts = append(conflicts, formatConflict(key, values))
				continue
			}
			defaults[key] = values[0].value
		}
	}
	walk(schemaRoot, "")

	if len(conflicts) != 0 {
		return nil, errors.New("the enabled products set conflicting values:\n  - " + strings.Join(conflicts, "\n  - "))
	}
	return defaults, nil
}

// productValues returns the values the enabled products set for a setting, following the order of 'enabled'
func productValues(setting map[string]interface{}, enabled []string) []productValue {
	productDefaults, _ := setting["product_defaults"].(map[string]interface{})
	productPlatformDefaults, _ := setting["product_platform_defaults"].(map[string]interface{})

	var values []productValue
	for _, product := range enabled {
		if value, found := productDefaults[product]; found {
			values = append(values, productValue{product: product, value: value})
			continue
		}
		if platformValues, ok := productPlatformDefaults[product].(map[string]interface{}); ok {
			// A product without value for the current platform doesn't change the setting
			if value := getPlatformDefault(platformValues); value != nil {
				values = append(values, productValue{product: product, value: value})
			}
		}
	}
	return values
}

func allEqual(values []productValue) bool {
	for _, v := range values[1:] {
		if !reflect.DeepEqual(v.value, values[0].value) {
			return false
		}
	}
	return true
}

func formatConflict(key string, values []productValue) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, fmt.Sprintf("%s=%v", v.product, v.value))
	}
	return key + ": " + strings.Join(parts, ", ")
}
