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
	"unicode"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners"
)

type variableGetter func(key []byte, svc listeners.Service) ([]byte, error)

var templateVariables = map[string]variableGetter{
	"host":     getHost,
	"pid":      getPid,
	"port":     getPort,
	"env":      getEnvvar,
	"hostname": getHostname,
}

// ConfigResolver resolves configuration against a given service
type ConfigResolver struct{}

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

	return resolvedConfig, nil
}

func getHost(tplVar []byte, svc listeners.Service) ([]byte, error) {
	hosts, err := svc.GetHosts()
	if err != nil {
		return nil, fmt.Errorf("failed to extract IP address for container %s, ignoring it. Source error: %s", svc.GetEntity(), err)
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("no network found for container %s, ignoring it", svc.GetEntity())
	}

	// a network was specified
	tplVarStr := string(tplVar)
	if ip, ok := hosts[tplVarStr]; ok {
		return []byte(ip), nil
	}
	log.Debugf("Network %q not found, trying bridge IP instead", tplVarStr)

	// otherwise use fallback policy
	ip, err := getFallbackHost(hosts)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve IP address for container %s, ignoring it. Source error: %s", svc.GetEntity(), err)
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
		return nil, fmt.Errorf("failed to extract port list for container %s, ignoring it. Source error: %s", svc.GetEntity(), err)
	} else if len(ports) == 0 {
		return nil, fmt.Errorf("no port found for container %s - ignoring it", svc.GetEntity())
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
		return nil, fmt.Errorf("port %s not found, skipping container %s", string(tplVar), svc.GetEntity())
	}
	if len(ports) <= idx {
		return nil, fmt.Errorf("index given for the port template var is too big, skipping container %s", svc.GetEntity())
	}
	return []byte(strconv.Itoa(ports[idx].Port)), nil
}

// getPid returns the process identifier of the service
func getPid(_ []byte, svc listeners.Service) ([]byte, error) {
	pid, err := svc.GetPid()
	if err != nil {
		return nil, fmt.Errorf("failed to get pid for service %s, skipping config - %s", svc.GetEntity(), err)
	}
	return []byte(strconv.Itoa(pid)), nil
}

// getHostname returns the hostname of the service, to be used
// when the IP is unavailable or erroneous
func getHostname(tplVar []byte, svc listeners.Service) ([]byte, error) {
	name, err := svc.GetHostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname for service %s, skipping config - %s", svc.GetEntity(), err)
	}
	return []byte(name), nil
}

// getEnvvar returns a system environment variable if found
func getEnvvar(tplVar []byte, svc listeners.Service) ([]byte, error) {
	if len(tplVar) == 0 {
		return nil, fmt.Errorf("envvar name is missing, skipping service %s", svc.GetEntity())
	}
	value, found := os.LookupEnv(string(tplVar))
	if !found {
		return nil, fmt.Errorf("failed to retrieve envvar %s, skipping service %s", tplVar, svc.GetEntity())
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
