// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package autodiscovery

import (
	"bytes"
	"fmt"
	"sync"
	"unicode"

	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/listeners"
	log "github.com/cihub/seelog"
)

type variableGetter func(key []byte, tpl check.Config, svc listeners.Service) []byte

var (
	templateVariables = map[string]variableGetter{
		"host":           getHost,
		"pid":            getPid,
		"port":           getPort,
		"container-name": getContainerName,
		"tags":           getOptTags,
	}
)

// ConfigResolver stores services and templates in cache, and matches
// services it hears about with templates to create valid configs.
// It is also responsible to send scheduling orders to AutoConfig
type ConfigResolver struct {
	ac              *AutoConfig
	collector       *collector.Collector
	templates       *TemplateCache
	services        map[listeners.ID]listeners.Service // Service.ID --> []Service
	serviceToChecks map[listeners.ID][]check.ID        // Service.ID --> []CheckID
	adIDToServices  map[string][]listeners.ID          // AD id --> services that have it
	newService      chan listeners.Service
	delService      chan listeners.Service
	stop            chan bool
	m               sync.Mutex
}

// NewConfigResolver returns a config resolver
func newConfigResolver(coll *collector.Collector, ac *AutoConfig, tc *TemplateCache) *ConfigResolver {
	cr := &ConfigResolver{
		ac:              ac,
		collector:       coll,
		templates:       tc,
		services:        make(map[listeners.ID]listeners.Service),
		serviceToChecks: make(map[listeners.ID][]check.ID, 0),
		adIDToServices:  make(map[string][]listeners.ID),
		newService:      make(chan listeners.Service),
		delService:      make(chan listeners.Service),
		stop:            make(chan bool),
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
				return
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
// identifiers in the `check.Config` struct to match a Service.
//
// The function might return more than one configuration for a single template,
// for example when the `ad_identifiers` section of a config.yaml file contains
// multiple entries, or when more than one Service has the same identifier,
// e.g. 'redis'.
//
// The function might return an empty list in the case the configuration has a
// list of Autodiscovery identifiers for services that are unknown to the
// resolver at this moment.
func (cr *ConfigResolver) ResolveTemplate(tpl check.Config) []check.Config {
	// use a map to dedupe configurations
	resolvedSet := map[string]check.Config{}

	// go through the AD identifiers provided by the template
	for _, id := range tpl.ADIdentifiers {
		// check out whether any service we know has this identifier
		serviceIds, found := cr.adIDToServices[id]
		if !found {
			log.Debugf("No service found with this AD identifier: %s", id)
			continue
		}

		for _, serviceID := range serviceIds {
			config, err := cr.resolve(tpl, cr.services[serviceID])
			if err == nil {
				resolvedSet[config.Digest()] = config
			} else {
				log.Debugf("Error resolving template %s for service %s: %v",
					config.Name, serviceID, err)
			}
		}
	}

	// build the slice of configs to return
	resolved := []check.Config{}
	for _, v := range resolvedSet {
		resolved = append(resolved, v)
	}

	return resolved
}

// resolve takes a template and a service and generates a config with
// valid connection info and relevant tags.
func (cr *ConfigResolver) resolve(tpl check.Config, svc listeners.Service) (check.Config, error) {
	vars := tpl.GetTemplateVariables()
	for _, v := range vars {
		name, key := parseTemplateVar(v)
		if f, ok := templateVariables[string(name)]; ok {
			resolvedVar := f(key, tpl, svc)
			if resolvedVar != nil {
				tpl.InitConfig = bytes.Replace(tpl.InitConfig, v, resolvedVar, -1)
				tpl.Instances[0] = bytes.Replace(tpl.Instances[0], v, resolvedVar, -1)
			}
		} else {
			return check.Config{}, fmt.Errorf("template variable %s does not exist", name)
		}
	}
	// TODO: call and add cr.getTags
	return tpl, nil
}

// processNewService takes a service, tries to match it against templates and
// triggers scheduling events if it finds a valid config for it.
func (cr *ConfigResolver) processNewService(svc listeners.Service) {
	cr.m.Lock()
	defer cr.m.Unlock()

	// in any case, register the service
	cr.services[svc.ID] = svc
	cr.serviceToChecks[svc.ID] = make([]check.ID, 0)

	// get all the templates matching service identifiers
	templates := []check.Config{}
	for _, adID := range svc.ADIdentifiers {
		// map the AD identifier to this service for reverse lookup
		cr.adIDToServices[adID] = append(cr.adIDToServices[adID], svc.ID)
		tpls, err := cr.templates.Get(adID)
		if err != nil {
			log.Errorf("Unable to fetch templates from the cache: %v", err)
		}
		templates = append(templates, tpls...)
	}

	for _, template := range templates {
		// resolve the template
		config, err := cr.resolve(template, svc)
		if err != nil {
			log.Errorf("Unable to resolve configuration template: %v", err)
			continue
		}

		// load the checks for this config using Autoconfig
		checks, err := cr.ac.GetChecks(config)
		if err != nil {
			log.Errorf("Unable to load the check: %v", err)
			continue
		}

		// ask the Collector to schedule the checks
		for _, check := range checks {
			id, err := cr.collector.RunCheck(check)
			if err != nil {
				log.Errorf("Unable to schedule the check: %v", err)
				continue
			}
			// add the check to the list of checks running against the service
			// this is used when a template or a service is removed
			// and we want to stop their related checks
			cr.serviceToChecks[svc.ID] = append(cr.serviceToChecks[svc.ID], id)
		}
	}
}

// processDelService takes a service, stops its associated checks, and updates the cache
func (cr *ConfigResolver) processDelService(svc listeners.Service) {
	cr.m.Lock()
	defer cr.m.Unlock()

	if checks, ok := cr.serviceToChecks[svc.ID]; ok {
		stopped := map[check.ID]struct{}{}
		for _, id := range checks {
			err := cr.collector.StopCheck(id)
			if err != nil {
				log.Errorf("Failed to stop check '%s': %s", id, err)
			}
			stopped[id] = struct{}{}
		}

		// remove the entry from `serviceToChecks`
		if len(stopped) == len(cr.serviceToChecks[svc.ID]) {
			// we managed to stop all the checks for this config
			delete(cr.serviceToChecks, svc.ID)
		} else {
			// keep the checks we failed to stop in `serviceToChecks[svc.ID]`
			dangling := []check.ID{}
			for _, id := range cr.serviceToChecks[svc.ID] {
				if _, found := stopped[id]; !found {
					dangling = append(dangling, id)
				}
			}
			cr.serviceToChecks[svc.ID] = dangling
		}
	}
}

// TODO (use svc.Hosts)
func getHost(tplVar []byte, tpl check.Config, svc listeners.Service) []byte {
	return []byte("127.0.0.1")
}

// TODO (use svc.Ports)
func getPort(tplVar []byte, tpl check.Config, svc listeners.Service) []byte {
	return []byte("80")
}

// TODO
func getPid(tplVar []byte, tpl check.Config, svc listeners.Service) []byte {
	return []byte("1")
}

// TODO
func getContainerName(tplVar []byte, tpl check.Config, svc listeners.Service) []byte {
	return []byte("test-container-name")
}

// getTags returns tags that are appended by default to all metrics.
// TODO (use svc.Tags)
func getTags(tplVar []byte, tpl check.Config, svc listeners.Service) []byte {
	return []byte("[\"tag:foo\", \"tag:bar\"]")
}

// getOptTags returns tags that are to be applied to templates with %%tags%%.
// This is generally reserved to high-cardinality tags that we want to provide,
// but not by default.
// TODO
func getOptTags(tplVar []byte, tpl check.Config, svc listeners.Service) []byte {
	return []byte("[\"opt:tag1\", \"opt:tag2\"]")
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
	split := bytes.Split(stripped, []byte("_"))
	name = split[0]
	if len(split) == 2 {
		key = split[1]
	} else {
		key = []byte("")
	}
	return name, key
}
