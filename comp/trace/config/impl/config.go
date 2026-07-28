// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package configimpl implements the trace-agent config component. This
// component temporarily wraps pkg/trace/config.
//
// This component initializes pkg/trace/config based on the bundle params, and
// will return the same results as that package. This is to support migration
// to a component architecture. When no code still uses pkg/trace/config, that
// package will be removed.
package configimpl

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"

	"go.yaml.in/yaml/v2"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	traceconfig "github.com/DataDog/datadog-agent/comp/trace/config/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pkgconfigutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	pkgtraceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// team: agent-apm

const (
	apiKeyConfigKey          = "api_key"
	apmConfigAPIKeyConfigKey = "apm_config.api_key" // deprecated setting

	// apmAdditionalEndpointsConfigKey, profilingAdditionalEndpointsConfigKey, and
	// evpAdditionalEndpointsConfigKey are the trace-relevant map-shape additional_endpoints
	// config keys that support DELA(...) dual-shipping directives (see
	// mapShapeAdditionalEndpointsConfigKeys in pkg/config/setup/config.go) and that setup.go
	// copies into the live trace AgentConfig once at startup. Delegated auth resolves a
	// DELA(...) directive at these keys asynchronously - if the initial synchronous exchange
	// fails, a background retry can succeed well after this component's Endpoints/
	// AdditionalEndpoints snapshot was built. Without a listener, that later success would
	// update core config but leave the trace-agent sending against the literal, never-resolved
	// DELA(...) string. See WIF-48.
	apmAdditionalEndpointsConfigKey       = "apm_config.additional_endpoints"
	profilingAdditionalEndpointsConfigKey = "apm_config.profiling_additional_endpoints"
	evpAdditionalEndpointsConfigKey       = "evp_proxy_config.additional_endpoints"
)

// Requires defines the trace config component deps.
// These include the core config configuration and component config params.
type Requires struct {
	Params traceconfig.Params
	Config coreconfig.Component
	Tagger tagger.Component
	IPC    ipc.Component
}

// Provides defines the output of the trace config component.
type Provides struct {
	Comp traceconfig.Component
}

// cfg implements the Component.
type cfg struct {
	// this component is currently implementing a thin wrapper around pkg/trace/config,
	// and uses globals in that package.
	*pkgtraceconfig.AgentConfig

	// coreConfig relates to the main agent config component
	coreConfig coreconfig.Component

	// warnings are the warnings generated during setup
	warnings *model.Warnings

	// UpdateAPIKeyFn is the callback func for API Key updates
	updateAPIKeyFn func(oldKey, newKey string)

	// ipc is used to retrieve the auth_token to issue authenticated requests
	ipc ipc.Component
}

// NewComponent is the default constructor for the component, it returns
// a component instance and an error.
func NewComponent(reqs Requires) (Provides, error) {
	tracecfg, err := setupConfigCommon(reqs)

	if err != nil {
		// Allow main Agent to start with missing API key
		if !(err == pkgtraceconfig.ErrMissingAPIKey && !reqs.Params.FailIfAPIKeyMissing) {
			return Provides{}, err
		}
	}

	c := cfg{
		AgentConfig: tracecfg,
		coreConfig:  reqs.Config,
		ipc:         reqs.IPC,
	}
	c.SetMaxMemCPU(env.IsContainerized())
	c.registerConfigUpdateListener()

	return Provides{Comp: &c}, nil
}

// registerConfigUpdateListener wires up c.coreConfig.OnUpdate so runtime config changes are
// reflected in the live trace AgentConfig without a restart: API key rotation (already handled
// before this refactor), and - see reloadAdditionalEndpoints - a trace-relevant
// additional_endpoints-shaped setting resolving asynchronously after this component's initial
// snapshot (e.g. a delegated auth DELA(...) directive whose background resolution succeeds after
// the synchronous exchange at startup failed). Shared by both NewComponent and NewMock so tests
// built via either constructor exercise the same reload behavior.
func (c *cfg) registerConfigUpdateListener() {
	c.coreConfig.OnUpdate(func(setting string, _ model.Source, oldValue, newValue any, _ uint64) {
		log.Debugf("OnUpdate: %s", setting)
		switch setting {
		case apiKeyConfigKey:
			if c.coreConfig.IsConfigured(apmConfigAPIKeyConfigKey) {
				// apm_config.api_key is deprecated. Since it overrides core api_key values during config setup,
				// if used, core API Key refresh is skipped. TODO: check usage of apm_config.api_key and remove it.
				log.Warn("cannot refresh api_key on trace-agent while `apm_config.api_key` is set. `apm_config.api_key` is deprecated, use core `api_key` instead")
				return
			}
			oldAPIKey, ok1 := oldValue.(string)
			newAPIKey, ok2 := newValue.(string)
			if ok1 && ok2 {
				log.Debugf("Updating API key in trace-agent config, replacing `%s` with `%s`", scrubber.HideKeyExceptLastChars(oldAPIKey), scrubber.HideKeyExceptLastChars(newAPIKey))
				// Update API Key on config, and propagate the signal to registered listeners
				newAPIKey = pkgconfigutils.SanitizeAPIKey(newAPIKey)
				c.updateAPIKey(oldAPIKey, newAPIKey)
			}
		case apmAdditionalEndpointsConfigKey, profilingAdditionalEndpointsConfigKey, evpAdditionalEndpointsConfigKey:
			// Reload the whole section rather than trying to patch a single entry: a map-shape
			// additional_endpoints update (e.g. a delegated auth API key rotation, or a background
			// DELA(...) resolution completing after this component's initial snapshot) doesn't tell
			// us which key changed, so the safest approach - mirroring
			// comp/forwarder/defaultforwarder/resolver.updateAdditionalEndpoints and
			// comp/logs/agent/config/endpoints.go's onConfigUpdateAdditionalEndpoints - is to
			// re-read the entire current value from config.
			c.reloadAdditionalEndpoints(setting)
		}
	})
}

// reloadAdditionalEndpoints re-derives the live trace AgentConfig field(s) backed by setting from
// the current core config, so a config change to a trace-relevant additional_endpoints-shaped
// value (see apmAdditionalEndpointsConfigKey and friends) - most notably a delegated auth
// background resolution completing after startup - takes effect without an agent restart.
func (c *cfg) reloadAdditionalEndpoints(setting string) {
	switch setting {
	case apmAdditionalEndpointsConfigKey:
		// c.Endpoints[0] is always the main endpoint, optionally followed by an MRF endpoint
		// (comp/trace/config/impl/setup.go's applyDatadogConfig) - both are populated before
		// apm_config.additional_endpoints is ever appended, and nothing appends to Endpoints
		// afterwards. Rebuilding just the tail (rather than the whole slice) preserves both of
		// those, including any API key rotation already applied to Endpoints[0].
		baseCount := 1
		if c.coreConfig.GetBool("multi_region_failover.enabled") {
			baseCount = 2
		}
		if len(c.Endpoints) < baseCount {
			log.Warnf("Cannot reload '%s': trace-agent has fewer endpoints (%d) than expected (%d)", setting, len(c.Endpoints), baseCount)
			return
		}
		base := make([]*pkgtraceconfig.Endpoint, baseCount)
		copy(base, c.Endpoints[:baseCount])
		c.Endpoints = appendEndpoints(base, apmAdditionalEndpointsConfigKey)

		// appendEndpoints doesn't set NoProxy - applyDatadogConfig (setup.go) does that in a
		// separate pass over the full Endpoints slice after building it. Without repeating that
		// pass here, endpoints rebuilt by this reload would silently lose their NoProxy flag and
		// start going through the proxy even if their Host is in proxy.no_proxy.
		if c.coreConfig.IsConfigured("proxy.no_proxy") {
			noProxy := make(map[string]bool)
			for _, host := range c.coreConfig.GetStringSlice("proxy.no_proxy") {
				noProxy[host] = true
			}
			for _, e := range c.Endpoints {
				e.NoProxy = noProxy[e.Host]
			}
		}
		log.Infof("Reloaded '%s' for trace-agent (%d additional endpoint(s))", setting, len(c.Endpoints)-baseCount)
	case profilingAdditionalEndpointsConfigKey:
		c.ProfilingProxy.AdditionalEndpoints = c.coreConfig.GetStringMapStringSlice(setting)
		log.Infof("Reloaded '%s' for trace-agent", setting)
	case evpAdditionalEndpointsConfigKey:
		c.EVPProxy.AdditionalEndpoints = c.coreConfig.GetStringMapStringSlice(setting)
		log.Infof("Reloaded '%s' for trace-agent", setting)
	}
}

func (c *cfg) updateAPIKey(oldKey, newKey string) {
	// Update API Key on config, and propagate the signal to registered listeners
	c.UpdateAPIKey(newKey)
	if c.updateAPIKeyFn != nil {
		c.updateAPIKeyFn(oldKey, newKey)
	}
}

// OnUpdateAPIKey registers a callback for API Key changes, only 1 callback can be used at a time
func (c *cfg) OnUpdateAPIKey(callback func(oldKey, newKey string)) {
	if c.updateAPIKeyFn != nil {
		log.Error("OnUpdateAPIKey has already been configured. Only 1 callback can be used at a time.")
	}
	c.updateAPIKeyFn = callback
}

func (c *cfg) Warnings() *model.Warnings {
	return c.warnings
}

func (c *cfg) Object() *pkgtraceconfig.AgentConfig {
	return c.AgentConfig
}

// SetHandler returns a handler to change the runtime configuration.
func (c *cfg) SetHandler() http.Handler {
	// HTTPMiddleware is used to ensure that the request is authenticated
	return c.ipc.HTTPMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Method != http.MethodPost {
				httpError(w, http.StatusMethodNotAllowed, fmt.Errorf("%s method not allowed, only %s", req.Method, http.MethodPost))
				return
			}
			for key, values := range req.URL.Query() {
				if len(values) == 0 {
					continue
				}
				value := html.UnescapeString(values[len(values)-1])
				switch key {
				case "log_level":
					lvl := strings.ToLower(value)
					if lvl == "warning" {
						lvl = "warn"
					}
					if err := pkgconfigutils.SetLogLevel(lvl, pkgconfigsetup.Datadog(), model.SourceAgentRuntime); err != nil {
						httpError(w, http.StatusInternalServerError, err)
						return
					}
					log.Infof("Switched log level to %s", lvl)
				default:
					log.Infof("Unsupported config change requested (key: %q).", key)
				}
			}
		}),
	)
}

// GetConfigHandler returns handler to get the runtime configuration.
func (c *cfg) GetConfigHandler() http.Handler {
	// HTTPMiddleware is used to ensure that the request is authenticated
	return c.ipc.HTTPMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Method != http.MethodGet {
				httpError(w,
					http.StatusMethodNotAllowed,
					fmt.Errorf("%s method not allowed, only %s", req.Method, http.MethodGet),
				)
				return
			}

			runtimeConfig, err := yaml.Marshal(c.coreConfig.AllSettings())
			if err != nil {
				log.Errorf("Unable to marshal runtime config response: %s", err)
				body, _ := json.Marshal(map[string]string{"error": err.Error()})
				http.Error(w, string(body), http.StatusInternalServerError)
				return
			}

			scrubbed, err := scrubber.ScrubYaml(runtimeConfig)
			if err != nil {
				log.Errorf("Unable to get the core config: %s", err)
				body, _ := json.Marshal(map[string]string{"error": err.Error()})
				http.Error(w, string(body), http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(scrubbed)
		}),
	)
}

// SetMaxMemCPU sets watchdog's max_memory and max_cpu_percent parameters.
// If the agent is containerized, max_memory and max_cpu_percent are disabled by default.
// Resource limits are better handled by container runtimes and orchestrators.
func (c *cfg) SetMaxMemCPU(isContainerized bool) {
	if c.coreConfig.Object().IsConfigured("apm_config.max_cpu_percent") {
		c.MaxCPU = c.coreConfig.Object().GetFloat64("apm_config.max_cpu_percent") / 100
	} else if isContainerized {
		log.Debug("Running in a container and apm_config.max_cpu_percent is not set, setting it to 0")
		c.MaxCPU = 0
	}

	if c.coreConfig.Object().IsConfigured("apm_config.max_memory") {
		c.MaxMemory = c.coreConfig.Object().GetFloat64("apm_config.max_memory")
	} else if isContainerized {
		log.Debug("Running in a container and apm_config.max_memory is not set, setting it to 0")
		c.MaxMemory = 0
	}
}
