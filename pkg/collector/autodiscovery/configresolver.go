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

var (
	templateVariables = map[string]func(key []byte, tpl check.Config, svc listeners.Service) []byte{
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
	ac               *AutoConfig
	collector        *collector.Collector
	templates        map[check.ID][]check.Config        // ConfigID --> []Config
	services         map[listeners.ID]listeners.Service // Service.ID --> []Service
	configToServices map[check.ID][]listeners.Service   // ConfigID --> []Service
	serviceToChecks  map[listeners.ID][]check.ID        // Service.ID --> []CheckID
	newService       chan listeners.Service
	delService       chan listeners.Service
	stop             chan bool
	m                sync.Mutex
}

// NewConfigResolver returns a config resolver
func NewConfigResolver(coll *collector.Collector, ac *AutoConfig, newSvc chan listeners.Service, delSvc chan listeners.Service) *ConfigResolver {
	tpls := make(map[check.ID][]check.Config)
	stop := make(chan bool)
	return &ConfigResolver{
		ac:               ac,
		collector:        coll,
		templates:        tpls,
		configToServices: make(map[check.ID][]listeners.Service, 0),
		serviceToChecks:  make(map[listeners.ID][]check.ID, 0),
		newService:       newSvc,
		delService:       delSvc,
		stop:             stop,
	}
}

// Listen waits on services and templates and process them as they come.
// It can trigger scheduling decisions using its AC reference or just update its cache.
func (cr *ConfigResolver) Listen() {
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

// processTemplates receives a template list, updates the ConfigResolver cache with them
// and may trigger scheduling events if new/deleted templates warrant it.
//
// TODO: right now processTemplates only works on a full template list.
// It needs to support partial updates
// TODO: move this to Autoconfig
// func (cr *ConfigResolver) processTemplates(tpls []check.Config) {
// 	tpls = removeDupeTemplates(tpls)
// 	cr.m.Lock()
// 	defer cr.m.Unlock()
// 	// init
// 	if len(cr.templates) == 0 {
// 		for _, t := range tpls {
// 			cr.templates[t.ID] = append(cr.templates[t.ID], t)
// 			cr.startChecksForTemplate(t)
// 		}
// 		// update
// 	} else {
// 		oldTpls := cr.templates
// 		cr.replaceInUseTemplates(tpls)
// 		newTpls := cr.templates
// 		// stop checks associated with templates that disappeared
// 		for id, templates := range oldTpls {
// 			if _, ok := newTpls[id]; !ok {
// 				for _, tpl := range templates {
// 					err := cr.stopChecksForTemplate(tpl)
// 					if err == nil {
// 						delete(cr.configToServices, tpl.ID)
// 					}
// 				}
// 			}
// 		}
// 	}
// }

// replaceInUseTemplates takes care of updating the template cache and
// running services when the ConfigResolver receives templates that were modified
// TODO: move this to Autoconfig
// func (cr *ConfigResolver) replaceInUseTemplates(tpls []check.Config) {
// 	for _, t := range tpls {
// 		if cachedTpls, ok := cr.templates[t.ID]; ok {
// 			addedInPlace := false
// 			for i, tpl := range cachedTpls {
// 				if tpl.Name == t.Name {
// 					cr.templates[t.ID][i] = t
// 					addedInPlace = true
// 					// it is an updated template, we need to update its checks
// 					cr.updateChecksForTemplate(t)
// 					break
// 				}
// 			}
// 			if addedInPlace == false {
// 				cr.templates[t.ID] = append(cachedTpls, t)
// 				// it is a new template, we need to try and run its checks
// 				cr.startChecksForTemplate(t)
// 			}
// 		} else {
// 			cr.templates[t.ID] = []check.Config{t}
// 			// in this case it is a new template for sure
// 			cr.startChecksForTemplate(t)
// 		}
// 	}
// }

// startChecksForTemplate takes a template, finds services that match it
// and schedules checks based on this matching
// func (cr *ConfigResolver) startChecksForTemplate(t check.Config) {
// 	for svcID := range cr.services {
// 		svc, ok := cr.services[svcID]
// 		if !ok {
// 			log.Debugf("Service %s doesn't exist, skipping", svcID)
// 			continue
// 		}
// 		if IsConfigMatching(svc.ConfigID, t.ID) {
// 			cr.configToServices[t.ID] = append(cr.configToServices[t.ID], svc)
// 			config, err := cr.ResolveConfig(t, svc)
// 			if err != nil {
// 				log.Errorf("Unable to generate a check config with template %s and service %s: %s", t.ID, svc.ConfigID, err)
// 				return
// 			}
// 			ids, err := cr.AC.LoadAndRun(config)
// 			if err == nil {
// 				for _, id := range ids {
// 					cr.serviceToChecks[svc.ID] = append(cr.serviceToChecks[svc.ID], id)
// 				}
// 			}
// 		}
// 	}
// 	return
// }

// stopChecksForTemplate takes a template, find running checks associated with it and stop them.
// func (cr *ConfigResolver) stopChecksForTemplate(t check.Config) error {
// 	if services, ok := cr.configToServices[t.ID]; ok {
// 		toDelete := make([]listeners.ID, 0)
// 		for _, svc := range services {
// 			if checks, ok := cr.serviceToChecks[svc.ID]; ok {
// 				stopFailure := false
// 				for _, check := range checks {
// 					err := cr.AC.StopCheck(check)
// 					if err != nil {
// 						log.Errorf("Failed to stop check '%s': %s", check, err)
// 						stopFailure = true
// 					}
// 				}
// 				if !stopFailure {
// 					toDelete = append(toDelete, svc.ID)
// 				}
// 			}
// 		}
// 		for _, s := range toDelete {
// 			delete(cr.serviceToChecks, s)
// 		}
// 	}
// 	return nil
// }

// updateChecksForTemplate takes a template, find running checks associated with it
// and update them with a fresh config based on the template's new version.
// func (cr *ConfigResolver) updateChecksForTemplate(t check.Config) {
// 	// TODO: does that actually work? the Config ID might not be unique.
// 	// What happens if several templates apply to the same services?
// 	// Maybe we need check name + Config.ID as a key?
// 	if services, ok := cr.configToServices[t.ID]; ok {
// 		for _, svc := range services {
// 			config, err := cr.ResolveConfig(t, svc)
// 			if err != nil {
// 				log.Errorf("Unable to generate a check config with template %s and service %s: %s", t.ID, svc.ConfigID, err)
// 				return
// 			}
// 			if checks, ok := cr.serviceToChecks[svc.ID]; ok {
// 				for _, check := range checks {
// 					err := cr.AC.ReloadCheck(check, config)
// 					if err != nil {
// 						log.Errorf("Failed to reload check '%s', previous config left as-is. Error: %s", check, err)
// 					}
// 				}
// 			} else {
// 				// this should not happen, but if by any chance we can
// 				// configure a check but not find the previous one
// 				// let's run it anyway?
// 				panic("TODO")
// 			}
// 		}
// 	}
// }

// ResolveTemplate attempts to resolve a configuration template using the AD
// identifiers in the `check.Config` struct to match a Service.
//
// The function might return more than one configuration for a single template,
// for example when the `ad_identifiers` section of a config.yaml file contains
// multiple entries.
//
// The function might return an empty list in the case the configuration has a
// list of Autodiscovery identifiers for services that are unknown to the
// resolver at this moment.
func (cr *ConfigResolver) ResolveTemplate(tpl check.Config) []check.Config {
	// use a map to dedupe configurations
	resolvedSet := map[string]check.Config{}
	resolved := []check.Config{}

	for _, id := range tpl.ADIdentifiers {
		// TODO: render the template using the services known by the resolver
		fmt.Println(id)
	}

	// build the slice of configs to return
	for _, v := range resolvedSet {
		resolved = append(resolved, v)
	}

	return resolved
}

// ResolveConfig takes a template and a service and generates a config with
// valid connection info and relevant tags.
func (cr *ConfigResolver) ResolveConfig(tpl check.Config, svc listeners.Service) (check.Config, error) {
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

	// // in any case, register the service
	// cr.services[svc.ID] = svc
	// cr.serviceToChecks[svc.ID] = make([]check.ID, 0)

	// for configID, tpls := range cr.templates {
	// 	if IsConfigMatching(svc.ConfigID, configID) {
	// 		// add svc to the list of services matching tpl
	// 		// this is used when a template is removed and we want to remove its related checks
	// 		cr.configToServices[configID] = append(cr.configToServices[configID], svc)

	// 		for _, tpl := range tpls {
	// 			// actually resolve the config and run the check
	// 			conf, err := cr.ResolveConfig(tpl, svc)
	// 			if err != nil {
	// 				log.Errorf("Unable to generate a check config with template %s and service %s: %s", tpl.Digest(), svc.ConfigID, err)
	// 			} else {
	// 				checkIDs, err := cr.AC.LoadAndRun(conf)
	// 				if err == nil {
	// 					for _, id := range checkIDs {
	// 						// add the check to the list of checks running against the service
	// 						// this is used when a template or a service is removed
	// 						// and we want to stop their related checks
	// 						cr.serviceToChecks[svc.ID] = append(cr.serviceToChecks[svc.ID], id)
	// 					}
	// 				} else {
	// 					log.Errorf("Failed to run check(s) based on config %s: %s", conf.Digest(), err)
	// 				}
	// 			}
	// 		}
	// 	}
	// }
}

// processDelService takes a service, stops its associated checks, and updates the cache
func (cr *ConfigResolver) processDelService(svc listeners.Service) {
	cr.m.Lock()
	defer cr.m.Unlock()

	if checks, ok := cr.serviceToChecks[svc.ID]; ok {
		stopFailure := false
		for _, check := range checks {
			err := cr.collector.StopCheck(check)
			if err != nil {
				log.Errorf("Failed to stop check '%s': %s", check, err)
				stopFailure = true
			}
		}
		if !stopFailure {
			delete(cr.serviceToChecks, svc.ID)
		}
	}
}

// removeDupeTemplates walks through a list of templates and removes duplicates
func removeDupeTemplates(tpls []check.Config) []check.Config {
	cleanedTpls := make([]check.Config, len(tpls))
	seenChecks := make(map[string]struct{}, len(tpls))

	for _, t := range tpls {
		d := t.Digest()
		if _, found := seenChecks[d]; found {
			log.Warnf("Duplicate template for resource %s and check %s. Using the first one only.", d, t.Name)
			continue
		}
		seenChecks[d] = struct{}{}
		cleanedTpls = append(cleanedTpls, t)
	}
	return cleanedTpls
}

// IsConfigMatching checks if a Service ConfigID and a config ID are a match
// TODO: decomp the Service ConfigID for more advanced matching
func IsConfigMatching(sID check.ID, tID check.ID) bool {
	if sID == tID {
		return true
	}
	return false
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
		if unicode.IsSpace(r) {
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
