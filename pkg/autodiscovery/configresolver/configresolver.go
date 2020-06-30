// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package configresolver

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
)

type variableGetter func(key []byte, svc listeners.Service) ([]byte, error)

var templateVariables = map[string]variableGetter{
	"host":     getHost,
	"pid":      getPid,
	"port":     getPort,
	"hostname": getHostname,
	"extra":    getExtra,
}

// SubstituteTemplateVariables replaces %%VARIABLES%% using the variableGetters passed in
func SubstituteTemplateVariables(config *integration.Config, getters map[string]variableGetter, svc listeners.Service) error {
	for i := 0; i < len(config.Instances); i++ {
		vars := config.GetTemplateVariablesForInstance(i)
		for _, v := range vars {
			if f, found := getters[string(v.Name)]; found {
				resolvedVar, err := f(v.Key, svc)
				if err != nil {
					return err
				}
				// init config vars are replaced by the first found
				config.InitConfig = bytes.Replace(config.InitConfig, v.Raw, resolvedVar, -1)
				config.Instances[i] = bytes.Replace(config.Instances[i], v.Raw, resolvedVar, -1)
			}
		}
	}
	return nil
}

// SubstituteTemplateEnvVars replaces %%ENV_VARIABLE%% from environment variables
func SubstituteTemplateEnvVars(config *integration.Config) error {
	var retErr error
	for i := 0; i < len(config.Instances); i++ {
		vars := config.GetTemplateVariablesForInstance(i)
		for _, v := range vars {
			if "env" == string(v.Name) {
				resolvedVar, err := getEnvvar(v.Key)
				if err != nil {
					log.Warnf("variable not replaced: %s", err)
					if retErr == nil {
						retErr = err
					}
					continue
				}
				// init config vars are replaced by the first found
				config.InitConfig = bytes.Replace(config.InitConfig, v.Raw, resolvedVar, -1)
				config.Instances[i] = bytes.Replace(config.Instances[i], v.Raw, resolvedVar, -1)
			}
		}
	}
	return retErr
}

// Resolve takes a template and a service and generates a config with
// valid connection info and relevant tags.
func Resolve(tpl integration.Config, svc listeners.Service) (integration.Config, error) {
	// Copy original template
	resolvedConfig := integration.Config{
		Name:            tpl.Name,
		Instances:       make([]integration.Data, len(tpl.Instances)),
		InitConfig:      make(integration.Data, len(tpl.InitConfig)),
		MetricConfig:    tpl.MetricConfig,
		LogsConfig:      tpl.LogsConfig,
		ADIdentifiers:   tpl.ADIdentifiers,
		ClusterCheck:    tpl.ClusterCheck,
		Provider:        tpl.Provider,
		Entity:          svc.GetEntity(),
		CreationTime:    svc.GetCreationTime(),
		NodeName:        tpl.NodeName,
		Source:          tpl.Source,
		MetricsExcluded: svc.HasFilter(containers.MetricsFilter),
		LogsExcluded:    svc.HasFilter(containers.LogsFilter),
	}
	copy(resolvedConfig.InitConfig, tpl.InitConfig)
	copy(resolvedConfig.Instances, tpl.Instances)

	// Ignore the config from file if it's overridden by an empty config
	// or by a different config for the same check
	if tpl.Provider == names.File && svc.GetCheckNames() != nil {
		checkNames := svc.GetCheckNames()
		lenCheckNames := len(checkNames)
		if lenCheckNames == 0 || (lenCheckNames == 1 && checkNames[0] == "") {
			// Empty check names on k8s annotations or docker labels override the check config from file
			// Used to deactivate unneeded OOTB autodiscovery checks defined in files
			// The checkNames slice is considered empty also if it contains one single empty string
			return resolvedConfig, fmt.Errorf("ignoring config from %s: another empty config is defined with the same AD identifier: %v", tpl.Source, tpl.ADIdentifiers)
		}
		for _, checkName := range checkNames {
			if tpl.Name == checkName {
				// Ignore config from file when the same check is activated on the same service via other config providers (k8s annotations or docker labels)
				return resolvedConfig, fmt.Errorf("ignoring config from %s: another config is defined for the check %s", tpl.Source, tpl.Name)
			}
		}

	}

	if resolvedConfig.IsCheckConfig() && !svc.IsReady() {
		return resolvedConfig, errors.New("unable to resolve, service not ready")
	}

	if err := SubstituteTemplateVariables(&resolvedConfig, templateVariables, svc); err != nil {
		return resolvedConfig, err
	}

	if err := SubstituteTemplateEnvVars(&resolvedConfig); err != nil {
		// We add the service name to the error here, since SubstituteTemplateEnvVars doesn't know about that
		return resolvedConfig, fmt.Errorf("%s, skipping service %s", err, svc.GetEntity())
	}

	if !tpl.IgnoreAutodiscoveryTags {
		if err := addServiceTags(&resolvedConfig, svc); err != nil {
			return resolvedConfig, fmt.Errorf("unable to add tags for service '%s', err: %s", svc.GetEntity(), err)
		}
	}

	return resolvedConfig, nil
}

func addServiceTags(resolvedConfig *integration.Config, svc listeners.Service) error {
	tags, err := svc.GetTags()
	if err != nil {
		return err
	}
	for i := 0; i < len(resolvedConfig.Instances); i++ {
		err = resolvedConfig.Instances[i].MergeAdditionalTags(tags)
		if err != nil {
			return err
		}
	}
	return nil
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

// getExtra returns specific data
func getExtra(tplVar []byte, svc listeners.Service) ([]byte, error) {
	value, err := svc.GetExtraConfig(tplVar)
	if err != nil {
		return nil, fmt.Errorf("failed to get extra info for service %s, skipping config - %s", svc.GetEntity(), err)
	}
	return value, nil
}

// getEnvvar returns a system environment variable if found
func getEnvvar(envVar []byte) ([]byte, error) {
	if len(envVar) == 0 {
		return nil, fmt.Errorf("envvar name is missing")
	}
	value, found := os.LookupEnv(string(envVar))
	if !found {
		return nil, fmt.Errorf("failed to retrieve envvar %s", envVar)
	}
	return []byte(value), nil
}
