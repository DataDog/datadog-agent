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
	"sync"

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

const additionalRemoteConfigClientsConfig = "cluster_agent.remote_configuration.additional_clients"

// remoteConfigClientPresets gives well-known client names a default product
// set, so a client that serves a whole subsystem does not have to spell out
// every product. An explicit "products" list always wins.
//
// Names match the configuration that enables each subsystem, and each preset
// covers exactly the products that subsystem resolves through
// ClientForProducts. Products with no such consumer are deliberately absent:
// an extra client is only created when something asks for one of its products,
// so giving it an unconsumed product would leave that product subscribed
// nowhere. K8S_INJECTION_DD is the current example -- it is subscribed via the
// default client only, and must stay there.
//
// The autoscaling preset deliberately includes the cluster autoscaling product:
// command.go resolves all enabled autoscaling products through a single
// ClientForProducts call, so leaving CLUSTER_AUTOSCALING_VALUES on the default
// client would split the subsystem across two clients and fail at startup the
// moment autoscaling.cluster.enabled is turned on.
var remoteConfigClientPresets = map[string][]string{
	// autoscaling.workload.enabled / autoscaling.cluster.enabled
	"autoscaling": {
		state.ProductContainerAutoscalingSettings,
		state.ProductContainerAutoscalingValues,
		state.ProductClusterAutoscalingValues,
	},
	// kubeactions.enabled
	"kubeactions": {
		state.ProductK8SActions,
	},
	// private_action_runner.enabled
	"private_action_runner": {
		state.ProductActionPlatformRunnerKeys,
	},
	// admission_controller.auto_instrumentation.patcher.enabled and
	// apm_config.instrumentation.on_demand. These are resolved by separate
	// ClientForProducts calls, so grouping them cannot split a subsystem.
	"apm_instrumentation": {
		state.ProductAPMTracing,
		state.ProductApmPolicies,
	},
}

func remoteConfigClientPresetNames() []string {
	names := make([]string, 0, len(remoteConfigClientPresets))
	for name := range remoteConfigClientPresets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type additionalRemoteConfigClientSpec struct {
	Name             string
	APIKey           string
	RCDDURL          string
	Site             string
	ConfigRoot       string
	DirectorRoot     string
	Key              string
	DatabaseFileName string
	Products         []string
}

type remoteConfigClientRegistry struct {
	mu sync.Mutex

	cfg            config.Component
	defaultService rccomp.Component
	hostnameGetter hostnameinterface.Component
	clusterName    string
	clusterID      string

	defaultInstance *remoteConfigClientInstance
	byProduct       map[string]*remoteConfigClientInstance
	clients         []*rcclient.Client
	pendingStart    []*rcclient.Client
	services        []*remoteconfig.CoreAgentService
}

type remoteConfigClientInstance struct {
	name     string
	spec     additionalRemoteConfigClientSpec
	products []string

	client  *rcclient.Client
	service *remoteconfig.CoreAgentService
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

	byProduct := make(map[string]*remoteConfigClientInstance)
	for _, spec := range specs {
		instance := &remoteConfigClientInstance{
			name:     spec.Name,
			spec:     spec,
			products: spec.Products,
		}
		for _, product := range spec.Products {
			byProduct[product] = instance
		}
	}

	// Whatever no additional client claims stays on the default client.
	defaultProducts := make([]string, 0, len(products))
	for _, product := range products {
		if _, ownedByExtraClient := byProduct[product]; !ownedByExtraClient {
			defaultProducts = append(defaultProducts, product)
		}
	}

	registry := &remoteConfigClientRegistry{
		cfg:            cfg,
		defaultService: defaultService,
		hostnameGetter: hostnameGetter,
		clusterName:    clusterName,
		clusterID:      clusterID,
		defaultInstance: &remoteConfigClientInstance{
			name:     "default",
			products: defaultProducts,
		},
		byProduct: byProduct,
	}

	return registry, nil
}

func (r *remoteConfigClientRegistry) DefaultClient() (*rcclient.Client, error) {
	if r == nil {
		return nil, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.clientForInstanceLocked(r.defaultInstance)
}

func (r *remoteConfigClientRegistry) ClientForProducts(products ...string) (*rcclient.Client, error) {
	if r == nil {
		return nil, nil
	}
	if len(products) == 0 {
		return r.DefaultClient()
	}

	var selectedInstance *remoteConfigClientInstance
	for _, product := range products {
		// Products without an explicit additional endpoint stay on the default
		// client, and therefore share the default remote-config service cache DB.
		productInstance := r.defaultInstance
		if instance, found := r.byProduct[product]; found {
			productInstance = instance
		}
		if selectedInstance == nil {
			selectedInstance = productInstance
			continue
		}
		if selectedInstance != productInstance {
			return nil, fmt.Errorf("remote config products %v are routed to different clients (%q and %q); configure products used by one subsystem on the same %s entry", products, selectedInstance.name, productInstance.name, additionalRemoteConfigClientsConfig)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	return r.clientForInstanceLocked(selectedInstance)
}

func (r *remoteConfigClientRegistry) clientForInstanceLocked(instance *remoteConfigClientInstance) (*rcclient.Client, error) {
	if instance == nil {
		return nil, nil
	}
	if instance.client != nil {
		return instance.client, nil
	}

	if instance == r.defaultInstance {
		client, err := initializeRemoteConfigClientWithRoots(r.defaultService, defaultRemoteConfigClientRoots(r.cfg), r.clusterName, r.clusterID, instance.products...)
		if err != nil {
			return nil, err
		}
		instance.client = client
		r.clients = append(r.clients, client)
		return client, nil
	}

	service, err := newAdditionalRemoteConfigService(r.cfg, r.hostnameGetter, instance.spec)
	if err != nil {
		return nil, err
	}

	client, err := initializeRemoteConfigClientWithRoots(service, additionalRemoteConfigClientRoots(r.cfg, instance.spec), r.clusterName, r.clusterID, instance.products...)
	if err != nil {
		_ = service.Stop()
		return nil, err
	}

	// Start the service, which only fills its own cache, but leave the client
	// stopped. The caller subscribes to its products after this returns, and the
	// client notifies only the listeners registered when an update arrives and
	// never replays: a client polling before its subsystem subscribes would
	// consume the current configuration and never deliver it. StartClients
	// starts them once every subsystem has wired up, mirroring how the default
	// client is started after subscribeAgentConfig/subscribeAgentTask.
	service.Start()

	instance.service = service
	instance.client = client
	r.services = append(r.services, service)
	r.clients = append(r.clients, client)
	r.pendingStart = append(r.pendingStart, client)
	return client, nil
}

// StartClients starts every additional client built so far. Call it once all
// subsystems have resolved their client and subscribed to their products.
func (r *remoteConfigClientRegistry) StartClients() {
	if r == nil {
		return
	}

	r.mu.Lock()
	pending := r.pendingStart
	r.pendingStart = nil
	r.mu.Unlock()

	for _, client := range pending {
		client.Start()
	}
}

func (r *remoteConfigClientRegistry) Close() {
	if r == nil {
		return
	}

	r.mu.Lock()
	clients := append([]*rcclient.Client(nil), r.clients...)
	services := append([]*remoteconfig.CoreAgentService(nil), r.services...)
	r.mu.Unlock()

	for _, client := range clients {
		client.Close()
	}
	for _, service := range services {
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
		remoteconfig.DefaultDatabaseFileName: "default",
	}
	for _, name := range names {
		rawSpec, ok := asStringMap(raw[name])
		if !ok {
			return nil, fmt.Errorf("%s.%s must be an object", additionalRemoteConfigClientsConfig, name)
		}

		spec := additionalRemoteConfigClientSpec{
			Name:             strings.TrimSpace(name),
			APIKey:           stringFromConfigMap(rawSpec, "api_key"),
			RCDDURL:          stringFromConfigMap(rawSpec, "rc_dd_url"),
			Site:             stringFromConfigMap(rawSpec, "site"),
			ConfigRoot:       stringFromConfigMap(rawSpec, "config_root"),
			DirectorRoot:     stringFromConfigMap(rawSpec, "director_root"),
			Key:              stringFromConfigMap(rawSpec, "key"),
			DatabaseFileName: stringFromConfigMap(rawSpec, "database_file_name"),
		}
		if spec.Name == "" {
			return nil, fmt.Errorf("%s contains an empty client name", additionalRemoteConfigClientsConfig)
		}
		if spec.RCDDURL == "" {
			return nil, fmt.Errorf("%s.%s.rc_dd_url must be set", additionalRemoteConfigClientsConfig, spec.Name)
		}
		if spec.APIKey == "" {
			return nil, fmt.Errorf("%s.%s.api_key must be set", additionalRemoteConfigClientsConfig, spec.Name)
		}
		// The client name selects the products it owns. There is no per-client
		// product list: a client that owned nothing would never be selected, and
		// routing an arbitrary product to a private endpoint is not something any
		// Cluster Agent subsystem asks for.
		preset, found := remoteConfigClientPresets[spec.Name]
		if !found {
			return nil, fmt.Errorf("%s.%s is not a known client name (known: %v)", additionalRemoteConfigClientsConfig, spec.Name, remoteConfigClientPresetNames())
		}
		spec.Products = append([]string(nil), preset...)
		var err error
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

func normalizeAdditionalRemoteConfigDatabaseFileName(specName, databaseFileName string) (string, error) {
	if databaseFileName == "" {
		databaseFileName = fmt.Sprintf("remote-config-%s.db", specName)
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
	apiKey := configUtils.SanitizeAPIKey(spec.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%s.%s API key is empty", additionalRemoteConfigClientsConfig, spec.Name)
	}

	site := additionalRemoteConfigClientRoots(cfg, spec).site

	options := []remoteconfig.Option{
		remoteconfig.WithAPIKey(apiKey),
		// Each client has its own static key; a process-wide api_key update
		// must not replace it.
		remoteconfig.WithoutAPIKeyUpdates(),
		remoteconfig.WithTraceAgentEnv(configUtils.GetTraceAgentDefaultEnv(cfg)),
		remoteconfig.WithDatabaseFileName(spec.DatabaseFileName),
		remoteconfig.WithConfigRootOverride(site, spec.ConfigRoot),
		remoteconfig.WithDirectorRootOverride(site, spec.DirectorRoot),
		remoteconfig.WithRcKey(spec.Key),
		remoteconfig.WithStatusInstance(spec.Name),
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

type noopRemoteConfigTelemetryReporter struct{}

func (noopRemoteConfigTelemetryReporter) IncRateLimit()                              {}
func (noopRemoteConfigTelemetryReporter) IncTimeout()                                {}
func (noopRemoteConfigTelemetryReporter) IncConfigSubscriptionsConnectedCounter()    {}
func (noopRemoteConfigTelemetryReporter) IncConfigSubscriptionsDisconnectedCounter() {}
func (noopRemoteConfigTelemetryReporter) SetConfigSubscriptionsActive(_ int)         {}
func (noopRemoteConfigTelemetryReporter) SetConfigSubscriptionClientsTracked(_ int)  {}
