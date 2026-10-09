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
//   - 'product_dependencies' maps every product to its dependencies (resolved recursively), its profiles and the
//     products it conflicts with. Profiles are alternative ways to run a product (ex: low resource usage); they're
//     enabled like products, and implicitly enable their product.
//
// Settings declare the value to use when a product is enabled with 'product_defaults' or
// 'product_platform_defaults'. Those values are written to the model.SourceProductEnablement layer, so any other
// source (file, env vars, ...) still overrides them.
//
// The schema definitions are validated by 'dda inv schema.lint', so the runtime only reports errors caused by the
// user configuration: unknown SKU or product, several profiles of a product, conflicting products, and products setting
// conflicting values.

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
	resolved, err := resolveProducts(sku, products, productDefinitions(coreRoot))
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

// productCatalog holds the product enablement definitions of the core schema
type productCatalog struct {
	// skus maps a SKU to the products it bundles
	skus map[string][]string
	// dependencies maps every product and profile to what it depends on; a profile depends on its product
	dependencies map[string][]string
	// profileOwners maps every profile to its product
	profileOwners map[string]string
	// conflicts maps every product and profile to the ones it can't be enabled with (symmetric)
	conflicts map[string]map[string]struct{}
}

// productDefinitions parses the 'sku_definitions' and 'product_dependencies' of a schema. Their format is enforced by
// 'dda inv schema.lint', malformed entries are ignored:
//
//	product:
//	  dependencies: [product or profile, ...]
//	  profiles:
//	    profile: [product or profile, ...]          # or {dependencies: [...], conflict: [...]}
//	  conflict: [product or profile, ...]
func productDefinitions(schemaRoot map[string]interface{}) productCatalog {
	catalog := productCatalog{
		skus:          map[string][]string{},
		dependencies:  map[string][]string{},
		profileOwners: map[string]string{},
		conflicts:     map[string]map[string]struct{}{},
	}
	addConflicts := func(name string, others []string) {
		for _, other := range others {
			for _, pair := range [][2]string{{name, other}, {other, name}} {
				if catalog.conflicts[pair[0]] == nil {
					catalog.conflicts[pair[0]] = map[string]struct{}{}
				}
				catalog.conflicts[pair[0]][pair[1]] = struct{}{}
			}
		}
	}

	skus, _ := schemaRoot["sku_definitions"].(map[string]interface{})
	for sku, products := range skus {
		catalog.skus[sku] = toStringSlice(products)
	}

	products, _ := schemaRoot["product_dependencies"].(map[string]interface{})
	for product, rawDefinition := range products {
		definition, ok := rawDefinition.(map[string]interface{})
		if !ok {
			continue
		}
		catalog.dependencies[product] = toStringSlice(definition["dependencies"])
		addConflicts(product, toStringSlice(definition["conflict"]))

		profiles, _ := definition["profiles"].(map[string]interface{})
		for profile, rawProfile := range profiles {
			// a profile is a list of dependencies, or a mapping {dependencies, conflict}
			dependencies := toStringSlice(rawProfile)
			if profileDefinition, ok := rawProfile.(map[string]interface{}); ok {
				dependencies = toStringSlice(profileDefinition["dependencies"])
				addConflicts(profile, toStringSlice(profileDefinition["conflict"]))
			}
			catalog.profileOwners[profile] = product
			catalog.dependencies[profile] = append([]string{product}, dependencies...)
		}
	}
	return catalog
}

// toStringSlice returns the strings of a YAML list
func toStringSlice(raw interface{}) []string {
	list, _ := raw.([]interface{})
	names := make([]string, 0, len(list))
	for _, item := range list {
		if name, ok := item.(string); ok {
			names = append(names, name)
		}
	}
	return names
}

// resolveProducts returns the sorted list of products and profiles enabled by a SKU and a list of products, including
// all their dependencies (a profile enables its product).
//
// It fails on unknown names, when more than one profile of the same product is selected, and when the enabled products
// can't be enabled together (see 'conflict' in the schema).
func resolveProducts(sku string, products []string, catalog productCatalog) ([]string, error) {
	requested := slices.Clone(products)
	selectedProfiles := map[string][]string{}
	for _, product := range products {
		if _, isSKU := catalog.skus[product]; isSKU {
			return nil, fmt.Errorf("'%s' is a SKU, it must be set with 'sku' instead of 'products'", product)
		}
		if _, found := catalog.dependencies[product]; !found {
			return nil, fmt.Errorf("unknown product '%s'", product)
		}
		if owner, isProfile := catalog.profileOwners[product]; isProfile && !slices.Contains(selectedProfiles[owner], product) {
			selectedProfiles[owner] = append(selectedProfiles[owner], product)
		}
	}
	for _, owner := range sortedKeys(selectedProfiles) {
		if profiles := selectedProfiles[owner]; len(profiles) > 1 {
			slices.Sort(profiles)
			return nil, fmt.Errorf("only one profile of '%s' can be selected, got: %s", owner, strings.Join(profiles, ", "))
		}
	}
	if sku != "" {
		skuProducts, found := catalog.skus[sku]
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
		requested = append(requested, catalog.dependencies[product]...)
	}

	resolved := sortedKeys(enabled)
	var conflicts []string
	for _, product := range resolved {
		for _, other := range sortedKeys(catalog.conflicts[product]) {
			if _, found := enabled[other]; found && product < other {
				conflicts = append(conflicts, fmt.Sprintf("'%s' and '%s' can't be enabled together", product, other))
			}
		}
	}
	if len(conflicts) != 0 {
		return nil, errors.New(strings.Join(conflicts, "; "))
	}
	return resolved, nil
}

// sortedKeys returns the sorted keys of a map
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// productValue is the value a product, or a combination of products, sets for a setting
type productValue struct {
	// product is the product name, or the names of a combination joined with '+'
	product string
	// products are the names the value applies to (one, or several for a combination)
	products []string
	value    interface{}
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

// productValues returns the values the enabled products set for a setting, sorted by key.
//
// Keys are product names, or combinations of products joined with '+' ('product_1+product_2') that only match when all
// their products are enabled. A matching combination replaces the values of the products it lists: values whose
// products are a strict subset of another matching key's products are dropped, so the most specific key wins.
func productValues(setting map[string]interface{}, enabled []string) []productValue {
	productDefaults, _ := setting["product_defaults"].(map[string]interface{})
	productPlatformDefaults, _ := setting["product_platform_defaults"].(map[string]interface{})
	enabledSet := make(map[string]struct{}, len(enabled))
	for _, product := range enabled {
		enabledSet[product] = struct{}{}
	}
	matches := func(key string) ([]string, bool) {
		products := strings.Split(key, "+")
		for _, product := range products {
			if _, found := enabledSet[product]; !found {
				return nil, false
			}
		}
		return products, true
	}

	var values []productValue
	for key, value := range productDefaults {
		if products, ok := matches(key); ok {
			values = append(values, productValue{product: key, products: products, value: value})
		}
	}
	for key, rawPlatformValues := range productPlatformDefaults {
		platformValues, isMap := rawPlatformValues.(map[string]interface{})
		products, ok := matches(key)
		if !isMap || !ok {
			continue
		}
		// A product (or combination) without value for the current platform doesn't change the setting
		if value := getPlatformDefault(platformValues); value != nil {
			values = append(values, productValue{product: key, products: products, value: value})
		}
	}

	var kept []productValue
	for _, candidate := range values {
		if !coveredByAnother(candidate, values) {
			kept = append(kept, candidate)
		}
	}
	slices.SortFunc(kept, func(a, b productValue) int { return strings.Compare(a.product, b.product) })
	return kept
}

// coveredByAnother reports whether the products of a value are a strict subset of the products of another value
func coveredByAnother(candidate productValue, values []productValue) bool {
	for _, other := range values {
		if len(other.products) <= len(candidate.products) {
			continue
		}
		covered := true
		for _, product := range candidate.products {
			if !slices.Contains(other.products, product) {
				covered = false
				break
			}
		}
		if covered {
			return true
		}
	}
	return false
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
