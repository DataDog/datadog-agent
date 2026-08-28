// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows && kubeapiserver

package start

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	rccomp "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/def"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	additionalRemoteConfigClientsConfig = "cluster_agent.remote_configuration.additional_clients"
	defaultRemoteConfigDatabaseFileName = "remote-config.db"
)

var processLevelRemoteConfigProducts = map[string]struct{}{
	state.ProductAgentConfig: {},
	state.ProductAgentTask:   {},
}

type additionalRemoteConfigClientSpec struct {
	Name             string
	APIKey           string
	APIKeySetting    string
	RCDDURL          string
	Site             string
	ConfigRoot       string
	DirectorRoot     string
	Key              string
	DatabaseFileName string
	Products         []string
}

type remoteConfigClientRegistry struct {
	defaultClient *rcclient.Client
	clients       []*rcclient.Client
	services      []*remoteconfig.CoreAgentService
	byProduct     map[string]*rcclient.Client
	productOwners map[string]string
}

type remoteConfigClientRoots struct {
	site         string
	directorRoot string
}

func initializeRemoteConfigClients(
	defaultService rccomp.Component,
	cfg config.Component,
	hostnameGetter hostnameinterface.Component,
	clusterName, clusterID string,
	products ...string,
) (*remoteConfigClientRegistry, error) {
	specs, err := getAdditionalRemoteConfigClientSpecs(cfg)
	if err != nil {
		return nil, err
	}

	extraProductOwners := make(map[string]string)
	for _, spec := range specs {
		for _, product := range spec.Products {
			if owner, found := extraProductOwners[product]; found {
				return nil, fmt.Errorf("%s: product %q is owned by both %q and %q", additionalRemoteConfigClientsConfig, product, owner, spec.Name)
			}
			extraProductOwners[product] = spec.Name
		}
	}

	defaultProducts := make([]string, 0, len(products))
	for _, product := range products {
		if _, ownedByExtraClient := extraProductOwners[product]; !ownedByExtraClient {
			defaultProducts = append(defaultProducts, product)
		}
	}

	defaultClient, err := initializeRemoteConfigClientWithRoots(defaultService, defaultRemoteConfigClientRoots(cfg), clusterName, clusterID, defaultProducts...)
	if err != nil {
		return nil, err
	}

	registry := &remoteConfigClientRegistry{
		defaultClient: defaultClient,
		clients:       []*rcclient.Client{defaultClient},
		byProduct:     make(map[string]*rcclient.Client),
		productOwners: make(map[string]string),
	}

	for _, spec := range specs {
		service, err := newAdditionalRemoteConfigService(cfg, hostnameGetter, spec)
		if err != nil {
			registry.Close()
			return nil, err
		}

		client, err := initializeRemoteConfigClientWithRoots(service, additionalRemoteConfigClientRoots(cfg, spec), clusterName, clusterID, spec.Products...)
		if err != nil {
			_ = service.Stop()
			registry.Close()
			return nil, err
		}

		service.Start()
		client.Start()

		registry.services = append(registry.services, service)
		registry.clients = append(registry.clients, client)
		for _, product := range spec.Products {
			registry.byProduct[product] = client
			registry.productOwners[product] = spec.Name
		}
	}

	return registry, nil
}

func (r *remoteConfigClientRegistry) DefaultClient() *rcclient.Client {
	if r == nil {
		return nil
	}
	return r.defaultClient
}

func (r *remoteConfigClientRegistry) ClientForProducts(products ...string) (*rcclient.Client, error) {
	if r == nil {
		return nil, nil
	}
	if len(products) == 0 {
		return r.defaultClient, nil
	}

	var selectedClient *rcclient.Client
	selectedOwner := "default"
	for _, product := range products {
		// Products without an explicit additional endpoint stay on the default
		// client, and therefore share the default remote-config service cache DB.
		productClient := r.defaultClient
		productOwner := "default"
		if client, found := r.byProduct[product]; found {
			productClient = client
			productOwner = r.productOwners[product]
		}
		if selectedClient == nil {
			selectedClient = productClient
			selectedOwner = productOwner
			continue
		}
		if selectedClient != productClient {
			return nil, fmt.Errorf("remote config products %v are routed to different clients (%q and %q); configure products used by one subsystem on the same %s entry", products, selectedOwner, productOwner, additionalRemoteConfigClientsConfig)
		}
	}

	return selectedClient, nil
}

func (r *remoteConfigClientRegistry) Close() {
	if r == nil {
		return
	}
	for _, client := range r.clients {
		client.Close()
	}
	for _, service := range r.services {
		if err := service.Stop(); err != nil {
			pkglog.Errorf("unable to stop additional remote config service: %s", err)
		}
	}
}

func getAdditionalRemoteConfigClientSpecs(cfg config.Component) ([]additionalRemoteConfigClientSpec, error) {
	raw := cfg.GetStringMap(additionalRemoteConfigClientsConfig)
	if len(raw) == 0 {
		return nil, nil
	}

	names := make([]string, 0, len(raw))
	for name := range raw {
		names = append(names, name)
	}
	sort.Strings(names)

	specs := make([]additionalRemoteConfigClientSpec, 0, len(names))
	databaseFileOwners := map[string]string{
		defaultRemoteConfigDatabaseFileName: "default",
	}
	for _, name := range names {
		rawSpec, ok := asStringMap(raw[name])
		if !ok {
			return nil, fmt.Errorf("%s.%s must be an object", additionalRemoteConfigClientsConfig, name)
		}

		spec := additionalRemoteConfigClientSpec{
			Name:             strings.TrimSpace(name),
			APIKey:           stringFromConfigMap(rawSpec, "api_key"),
			APIKeySetting:    stringFromConfigMap(rawSpec, "api_key_setting"),
			RCDDURL:          stringFromConfigMap(rawSpec, "rc_dd_url"),
			Site:             stringFromConfigMap(rawSpec, "site"),
			ConfigRoot:       stringFromConfigMap(rawSpec, "config_root"),
			DirectorRoot:     stringFromConfigMap(rawSpec, "director_root"),
			Key:              stringFromConfigMap(rawSpec, "key"),
			DatabaseFileName: stringFromConfigMap(rawSpec, "database_file_name"),
			Products:         stringSliceFromConfigMap(rawSpec, "products"),
		}
		if spec.Name == "" {
			return nil, fmt.Errorf("%s contains an empty client name", additionalRemoteConfigClientsConfig)
		}
		if spec.RCDDURL == "" {
			return nil, fmt.Errorf("%s.%s.rc_dd_url must be set", additionalRemoteConfigClientsConfig, spec.Name)
		}
		if spec.APIKey == "" && spec.APIKeySetting == "" {
			return nil, fmt.Errorf("%s.%s must set api_key or api_key_setting", additionalRemoteConfigClientsConfig, spec.Name)
		}
		if len(spec.Products) == 0 {
			return nil, fmt.Errorf("%s.%s.products must contain at least one product", additionalRemoteConfigClientsConfig, spec.Name)
		}
		if err := validateAdditionalRemoteConfigProducts(spec); err != nil {
			return nil, err
		}
		spec.DatabaseFileName, err = normalizeAdditionalRemoteConfigDatabaseFileName(spec.Name, spec.DatabaseFileName)
		if err != nil {
			return nil, err
		}
		if owner, found := databaseFileOwners[spec.DatabaseFileName]; found {
			return nil, fmt.Errorf("%s.%s.database_file_name %q is already used by %q", additionalRemoteConfigClientsConfig, spec.Name, spec.DatabaseFileName, owner)
		}
		databaseFileOwners[spec.DatabaseFileName] = spec.Name
		specs = append(specs, spec)
	}

	return specs, nil
}

func validateAdditionalRemoteConfigProducts(spec additionalRemoteConfigClientSpec) error {
	seen := make(map[string]struct{}, len(spec.Products))
	for _, product := range spec.Products {
		if _, blocked := processLevelRemoteConfigProducts[product]; blocked {
			return fmt.Errorf("%s.%s.products cannot contain process-level product %q", additionalRemoteConfigClientsConfig, spec.Name, product)
		}
		if _, found := seen[product]; found {
			return fmt.Errorf("%s.%s.products contains duplicate product %q", additionalRemoteConfigClientsConfig, spec.Name, product)
		}
		seen[product] = struct{}{}
	}
	return nil
}

func normalizeAdditionalRemoteConfigDatabaseFileName(specName, databaseFileName string) (string, error) {
	if databaseFileName == "" {
		databaseFileName = fmt.Sprintf("remote-config-%s.db", safeRemoteConfigInstanceName(specName))
	}
	if databaseFileName == "." || databaseFileName == ".." || strings.ContainsAny(databaseFileName, `/\`) {
		return "", fmt.Errorf("%s.%s.database_file_name must be a basename, got %q", additionalRemoteConfigClientsConfig, specName, databaseFileName)
	}
	return databaseFileName, nil
}

func newAdditionalRemoteConfigService(
	cfg config.Component,
	hostnameGetter hostnameinterface.Component,
	spec additionalRemoteConfigClientSpec,
) (*remoteconfig.CoreAgentService, error) {
	apiKey := spec.APIKey
	if apiKey == "" && spec.APIKeySetting != "" {
		apiKey = cfg.GetString(spec.APIKeySetting)
	}
	apiKey = configUtils.SanitizeAPIKey(apiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%s.%s API key is empty", additionalRemoteConfigClientsConfig, spec.Name)
	}

	site := additionalRemoteConfigClientRoots(cfg, spec).site

	options := []remoteconfig.Option{
		remoteconfig.WithAPIKey(apiKey),
		remoteconfig.WithAPIKeyUpdateSetting(spec.APIKeySetting),
		remoteconfig.WithTraceAgentEnv(configUtils.GetTraceAgentDefaultEnv(cfg)),
		remoteconfig.WithDatabaseFileName(spec.DatabaseFileName),
		remoteconfig.WithConfigRootOverride(site, spec.ConfigRoot),
		remoteconfig.WithDirectorRootOverride(site, spec.DirectorRoot),
		remoteconfig.WithRcKey(spec.Key),
		remoteconfig.WithStatusInstance("cluster-agent:" + spec.Name),
	}

	service, err := remoteconfig.NewService(
		cfg,
		"Remote Config "+spec.Name,
		spec.RCDDURL,
		hostnameGetter.GetSafe(context.Background()),
		getClusterAgentRemoteConfigTags(cfg),
		noopRemoteConfigTelemetryReporter{},
		version.AgentVersion,
		options...,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create %s.%s: %w", additionalRemoteConfigClientsConfig, spec.Name, err)
	}

	return service, nil
}

func defaultRemoteConfigClientRoots(cfg config.Component) remoteConfigClientRoots {
	return remoteConfigClientRoots{
		site:         cfg.GetString("site"),
		directorRoot: cfg.GetString("remote_configuration.director_root"),
	}
}

func additionalRemoteConfigClientRoots(cfg config.Component, spec additionalRemoteConfigClientSpec) remoteConfigClientRoots {
	site := spec.Site
	if site == "" {
		site = cfg.GetString("site")
	}
	return remoteConfigClientRoots{
		site:         site,
		directorRoot: spec.DirectorRoot,
	}
}

func getClusterAgentRemoteConfigTags(cfg config.Component) func() []string {
	return func() []string {
		tags := configUtils.GetConfiguredTags(cfg, false)
		return append(tags, configUtils.GetConfiguredDCATags(cfg)...)
	}
}

func safeRemoteConfigInstanceName(name string) string {
	var builder strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			builder.WriteRune(r)
		} else {
			builder.WriteRune('_')
		}
	}
	safeName := strings.Trim(builder.String(), "._-")
	if safeName == "" {
		return "extra"
	}
	return safeName
}

func asStringMap(value interface{}) (map[string]interface{}, bool) {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed, true
	case map[interface{}]interface{}:
		converted := make(map[string]interface{}, len(typed))
		for key, value := range typed {
			keyString, ok := key.(string)
			if !ok {
				return nil, false
			}
			converted[keyString] = value
		}
		return converted, true
	default:
		return nil, false
	}
}

func stringFromConfigMap(raw map[string]interface{}, key string) string {
	value, found := raw[key]
	if !found || value == nil {
		return ""
	}
	if stringValue, ok := value.(string); ok {
		return strings.TrimSpace(stringValue)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func stringSliceFromConfigMap(raw map[string]interface{}, key string) []string {
	value, found := raw[key]
	if !found || value == nil {
		return nil
	}

	var rawValues []string
	switch typed := value.(type) {
	case []string:
		rawValues = typed
	case []interface{}:
		rawValues = make([]string, 0, len(typed))
		for _, entry := range typed {
			rawValues = append(rawValues, fmt.Sprint(entry))
		}
	default:
		rawValues = []string{fmt.Sprint(typed)}
	}

	values := make([]string, 0, len(rawValues))
	for _, rawValue := range rawValues {
		value := strings.TrimSpace(rawValue)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

type noopRemoteConfigTelemetryReporter struct{}

func (noopRemoteConfigTelemetryReporter) IncRateLimit()                              {}
func (noopRemoteConfigTelemetryReporter) IncTimeout()                                {}
func (noopRemoteConfigTelemetryReporter) IncConfigSubscriptionsConnectedCounter()    {}
func (noopRemoteConfigTelemetryReporter) IncConfigSubscriptionsDisconnectedCounter() {}
func (noopRemoteConfigTelemetryReporter) SetConfigSubscriptionsActive(_ int)         {}
func (noopRemoteConfigTelemetryReporter) SetConfigSubscriptionClientsTracked(_ int)  {}
