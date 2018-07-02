// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package autodiscovery

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

type variableGetter func(key []byte, svc listeners.Service) ([]byte, error)

var (
	templateVariables = map[string]variableGetter{
		"host":     getHost,
		"pid":      getPid,
		"port":     getPort,
		"env":      getEnvvar,
		"hostname": getHostname,
	}
)

// ConfigResolver stores services and templates in cache, and matches
// services it hears about with templates to create valid configs.
// It is also responsible to send scheduling orders to AutoConfig
type ConfigResolver struct {
	ac              *AutoConfig
	collector       *collector.Collector
	templates       *TemplateCache
	services        map[listeners.ID]listeners.Service // Service.ID --> Service
	adIDToServices  map[string]map[listeners.ID]bool   // AD id --> services that have it
	configToService map[string]listeners.ID            // config digest --> service ID
	newService      chan listeners.Service
	delService      chan listeners.Service
	stop            chan bool
	health          *health.Handle
	m               sync.Mutex
}

// NewConfigResolver returns a config resolver
func newConfigResolver(coll *collector.Collector, ac *AutoConfig, tc *TemplateCache) *ConfigResolver {
	cr := &ConfigResolver{
		ac:              ac,
		collector:       coll,
		templates:       tc,
		services:        make(map[listeners.ID]listeners.Service),
		adIDToServices:  make(map[string]map[listeners.ID]bool),
		configToService: make(map[string]listeners.ID),
		newService:      make(chan listeners.Service),
		delService:      make(chan listeners.Service),
		stop:            make(chan bool),
		health:          health.Register("ad-configresolver"),
	}

	// start listening
	cr.listen()

	return cr
}

// listen waits on services and templates and process them as they come.
// It can trigger scheduling decisions using its AC reference or just update its cache.
func (cr *ConfigResolver) listen() {
	go func() {
		for {
			select {
			case <-cr.stop:
				cr.health.Deregister()
				return
			case <-cr.health.C:
			case svc := <-cr.newService:
				cr.processNewService(svc)
			case svc := <-cr.delService:
				cr.processDelService(svc)
			}
		}
	}()
}

// Stop shuts down the config resolver
func (cr *ConfigResolver) Stop() {
	cr.stop <- true
}

// ResolveTemplate attempts to resolve a configuration template using the AD
// identifiers in the `integration.Config` struct to match a Service.
//
// The function might return more than one configuration for a single template,
// for example when the `ad_identifiers` section of a config.yaml file contains
// multiple entries, or when more than one Service has the same identifier,
// e.g. 'redis'.
//
// The function might return an empty list in the case the configuration has a
// list of Autodiscovery identifiers for services that are unknown to the
// resolver at this moment.
func (cr *ConfigResolver) ResolveTemplate(tpl integration.Config) []integration.Config {
	// use a map to dedupe configurations
	// FIXME: the config digest as the key is currently not reliable
	resolvedSet := map[string]integration.Config{}

	// go through the AD identifiers provided by the template
	for _, id := range tpl.ADIdentifiers {
		// check out whether any service we know has this identifier
		serviceIds, found := cr.adIDToServices[id]
		if !found {
			s := fmt.Sprintf("No service found with this AD identifier: %s", id)
			errorStats.setResolveWarning(tpl.Name, s)
			log.Debugf(s)
			continue
		}

		for serviceID := range serviceIds {
			config, err := cr.resolve(tpl, cr.services[serviceID])
			if err == nil {
				resolvedSet[config.Digest()] = config
				continue
			}
			err = fmt.Errorf("error resolving template %s for service %s: %v", tpl.Name, serviceID, err)
			errorStats.setResolveWarning(tpl.Name, err.Error())
			log.Warn(err)
		}
	}

	// build the slice of configs to return
	var resolved []integration.Config
	for _, v := range resolvedSet {
		resolved = append(resolved, v)
	}

	return resolved
}

// resolve takes a template and a service and generates a config with
// valid connection info and relevant tags.
func (cr *ConfigResolver) resolve(tpl integration.Config, svc listeners.Service) (integration.Config, error) {
	// Copy original template
	resolvedConfig := integration.Config{
		Name:          tpl.Name,
		Instances:     make([]integration.Data, len(tpl.Instances)),
		InitConfig:    make(integration.Data, len(tpl.InitConfig)),
		MetricConfig:  tpl.MetricConfig,
		ADIdentifiers: tpl.ADIdentifiers,
		Provider:      tpl.Provider,
	}
	copy(resolvedConfig.InitConfig, tpl.InitConfig)
	copy(resolvedConfig.Instances, tpl.Instances)

	tags, err := svc.GetTags()
	if err != nil {
		return resolvedConfig, err
	}
	for i := 0; i < len(tpl.Instances); i++ {
		// Copy original content from template
		vars := tpl.GetTemplateVariablesForInstance(i)
		for _, v := range vars {
			name, key := parseTemplateVar(v)
			if f, found := templateVariables[string(name)]; found {
				resolvedVar, err := f(key, svc)
				if err != nil {
					return integration.Config{}, err
				}
				// init config vars are replaced by the first found
				resolvedConfig.InitConfig = bytes.Replace(resolvedConfig.InitConfig, v, resolvedVar, -1)
				resolvedConfig.Instances[i] = bytes.Replace(resolvedConfig.Instances[i], v, resolvedVar, -1)
			}
		}
		err = resolvedConfig.Instances[i].MergeAdditionalTags(tags)
		if err != nil {
			return resolvedConfig, err
		}
	}

	// store resolved configs in the AC
	cr.ac.store.setLoadedConfig(resolvedConfig)
	cr.ac.store.addConfigForService(svc.GetID(), resolvedConfig)
	cr.configToService[resolvedConfig.Digest()] = svc.GetID()
	// TODO: harmonize service & entities ID
	entityName := string(svc.GetID())
	if !strings.Contains(entityName, "://") {
		entityName = docker.ContainerIDToEntityName(entityName)
	}
	cr.ac.store.setTagsHashForService(
		svc.GetID(),
		tagger.GetEntityHash(entityName),
	)

	return resolvedConfig, nil
}

// processNewService takes a service, tries to match it against templates and
// triggers scheduling events if it finds a valid config for it.
func (cr *ConfigResolver) processNewService(svc listeners.Service) {
	cr.m.Lock()
	defer cr.m.Unlock()

	// in any case, register the service and store its tag hash
	cr.services[svc.GetID()] = svc

	// get all the templates matching service identifiers
	var templates []integration.Config
	ADIdentifiers, err := svc.GetADIdentifiers()
	if err != nil {
		log.Errorf("Failed to get AD identifiers for service %s, it will not be monitored - %s", svc.GetID(), err)
		return
	}
	for _, adID := range ADIdentifiers {
		// map the AD identifier to this service for reverse lookup
		if cr.adIDToServices[adID] == nil {
			cr.adIDToServices[adID] = make(map[listeners.ID]bool)
		}
		cr.adIDToServices[adID][svc.GetID()] = true
		tpls, err := cr.templates.Get(adID)
		if err != nil {
			log.Debugf("Unable to fetch templates from the cache: %v", err)
		}
		templates = append(templates, tpls...)
	}

	for _, template := range templates {
		// resolve the template
		config, err := cr.resolve(template, svc)
		if err != nil {
			s := fmt.Sprintf("Unable to resolve configuration template: %v", err)
			errorStats.setResolveWarning(template.Name, s)
			log.Errorf(s)
			continue
		}
		errorStats.removeResolveWarnings(config.Name)

		// load the checks for this config using Autoconfig
		checks := cr.ac.getChecksFromConfigs([]integration.Config{config}, true)

		// ask the Collector to schedule the checks
		cr.ac.schedule(checks)
	}
}

// processDelService takes a service, stops its associated checks, and updates the cache
func (cr *ConfigResolver) processDelService(svc listeners.Service) {
	cr.m.Lock()
	defer cr.m.Unlock()

	delete(cr.services, svc.GetID())
	configs := cr.ac.store.getConfigsForService(svc.GetID())
	cr.ac.store.removeConfigsForService(svc.GetID())
	cr.ac.processRemovedConfigs(configs)
	cr.ac.store.removeTagsHashForService(svc.GetID())
}

func getHost(tplVar []byte, svc listeners.Service) ([]byte, error) {
	hosts, err := svc.GetHosts()
	if err != nil {
		return nil, fmt.Errorf("failed to extract IP address for container %s, ignoring it. Source error: %s", svc.GetID(), err)
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("no network found for container %s, ignoring it", svc.GetID())
	}

	// a network was specified
	tplVarStr := string(tplVar)
	if ip, ok := hosts[tplVarStr]; ok {
		return []byte(ip), nil
	}
	log.Warnf("Network %q not found, trying bridge IP instead", tplVarStr)

	// otherwise use fallback policy
	ip, err := getFallbackHost(hosts)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve IP address for container %s, ignoring it. Source error: %s", svc.GetID(), err)
	}

	return []byte(ip), nil
}

// getFallbackHost implements the fallback strategy to get a service's IP address
// the current strategy is:
// 		- if there's only one network we use its IP
// 		- otherwise we look for the bridge net and return its IP address
// 		- if we can't find it we fail because we shouldn't try and guess the IP address
func getFallbackHost(hosts map[string]string) (string, error) {
	if len(hosts) == 1 {
		for _, host := range hosts {
			return host, nil
		}
	}
	for k, v := range hosts {
		if k == "bridge" {
			return v, nil
		}
	}
	return "", errors.New("not able to determine which network is reachable")
}

// getPort returns ports of the service
func getPort(tplVar []byte, svc listeners.Service) ([]byte, error) {
	ports, err := svc.GetPorts()
	if err != nil {
		return nil, fmt.Errorf("failed to extract port list for container %s, ignoring it. Source error: %s", svc.GetID(), err)
	} else if len(ports) == 0 {
		return nil, fmt.Errorf("no port found for container %s - ignoring it", svc.GetID())
	}

	if len(tplVar) == 0 {
		return []byte(strconv.Itoa(ports[len(ports)-1].Port)), nil
	}

	idx, err := strconv.Atoi(string(tplVar))
	if err != nil {
		// The template variable is not an index so try to lookup port by name.
		for _, port := range ports {
			if port.Name == string(tplVar) {
				return []byte(strconv.Itoa(port.Port)), nil
			}
		}
		return nil, fmt.Errorf("port %s not found, skipping container %s", string(tplVar), svc.GetID())
	}
	if len(ports) <= idx {
		return nil, fmt.Errorf("index given for the port template var is too big, skipping container %s", svc.GetID())
	}
	return []byte(strconv.Itoa(ports[idx].Port)), nil
}

// getPid returns the process identifier of the service
func getPid(_ []byte, svc listeners.Service) ([]byte, error) {
	pid, err := svc.GetPid()
	if err != nil {
		return nil, fmt.Errorf("failed to get pid for service %s, skipping config - %s", svc.GetID(), err)
	}
	return []byte(strconv.Itoa(pid)), nil
}

// getHostname returns the hostname of the service, to be used
// when the IP is unavailable or erroneous
func getHostname(tplVar []byte, svc listeners.Service) ([]byte, error) {
	name, err := svc.GetHostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname for service %s, skipping config - %s", svc.GetID(), err)
	}
	return []byte(name), nil
}

// getEnvvar returns a system environment variable if found
func getEnvvar(tplVar []byte, svc listeners.Service) ([]byte, error) {
	if len(tplVar) == 0 {
		return nil, fmt.Errorf("envvar name is missing, skipping service %s", svc.GetID())
	}
	value, found := os.LookupEnv(string(tplVar))
	if !found {
		return nil, fmt.Errorf("failed to retrieve envvar %s, skipping service %s", tplVar, svc.GetID())
	}
	return []byte(value), nil
}

// parseTemplateVar extracts the name of the var
// and the key (or index if it can be cast to an int)
func parseTemplateVar(v []byte) (name, key []byte) {
	stripped := bytes.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '%' {
			return -1
		}
		return r
	}, v)
	parts := bytes.SplitN(stripped, []byte("_"), 2)
	name = parts[0]
	if len(parts) == 2 {
		return name, parts[1]
	}
	return name, []byte("")
}
