// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tmplvar

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ContainerPort represents a network port in a container
type ContainerPort struct {
	Port int
	Name string
}

// TemplateContext provides the runtime metadata needed to resolve template variables
type TemplateContext interface {
	// GetServiceID returns a unique identifier for this service (for error messages)
	GetServiceID() string

	// GetHosts returns a map of network name to IP address
	GetHosts() (map[string]string, error)

	// GetPorts returns the list of exposed ports
	GetPorts() ([]ContainerPort, error)

	// GetPid returns the process ID
	GetPid() (int, error)

	// GetHostname returns the hostname
	GetHostname() (string, error)

	// GetExtraConfig returns listener-specific configuration (for %%kube_*%% and %%extra_*%%)
	GetExtraConfig(key string) (string, error)
}

var varPattern = regexp.MustCompile(`%%(.+?)(?:_(.+?))?%%`)

// ResolveString resolves template variables in a string
// Supported variables: %%host%%, %%port%%, %%pid%%, %%hostname%%, %%env_VAR%%, %%kube_*%%, %%extra_*%%
func ResolveString(in string, ctx TemplateContext) (string, error) {
	varIndexes := varPattern.FindAllStringSubmatchIndex(in, -1)

	if len(varIndexes) == 0 {
		return in, nil
	}

	var sb strings.Builder
	var lastErr error

	sb.WriteString(in[:varIndexes[0][0]])
	for i := range varIndexes {
		if i != 0 {
			sb.WriteString(in[varIndexes[i-1][1]:varIndexes[i][0]])
		}

		varName := in[varIndexes[i][2]:varIndexes[i][3]]
		varKey := ""
		if varIndexes[i][4] != -1 {
			varKey = in[varIndexes[i][4]:varIndexes[i][5]]
		}

		resolvedVar, err := resolveVariable(varName, varKey, ctx)
		if err != nil {
			// Store error but continue trying to resolve other variables
			lastErr = err
			log.Debugf("Failed to resolve %%%s%%: %v", varName, err)
			// Keep the original template variable in the output
			sb.WriteString("%%")
			sb.WriteString(varName)
			if varKey != "" {
				sb.WriteString("_")
				sb.WriteString(varKey)
			}
			sb.WriteString("%%")
		} else {
			sb.WriteString(resolvedVar)
		}
	}
	sb.WriteString(in[varIndexes[len(varIndexes)-1][1]:])

	return sb.String(), lastErr
}

func resolveVariable(varName, varKey string, ctx TemplateContext) (string, error) {
	switch varName {
	case "host":
		return getHost(varKey, ctx)
	case "port":
		return getPort(varKey, ctx)
	case "pid":
		return getPid(varKey, ctx)
	case "hostname":
		return getHostname(varKey, ctx)
	case "env":
		return getEnvvar(varKey, ctx)
	case "kube", "extra":
		return getExtraConfig(varKey, ctx)
	default:
		if ctx != nil {
			return "", fmt.Errorf("invalid %%%s%% tag for service '%s'", varName, ctx.GetServiceID())
		}
		return "", fmt.Errorf("invalid %%%s%% tag", varName)
	}
}

func getHost(tplVar string, ctx TemplateContext) (string, error) {
	if ctx == nil {
		return "", errors.New("no context available for %%host%% resolution")
	}

	hosts, err := ctx.GetHosts()
	if err != nil {
		return "", fmt.Errorf("failed to extract IP address for %s: %w", ctx.GetServiceID(), err)
	}
	if len(hosts) == 0 {
		return "", fmt.Errorf("no network found for %s", ctx.GetServiceID())
	}

	// A network was specified
	if ip, ok := hosts[tplVar]; ok {
		return ip, nil
	}

	// Use fallback policy
	if len(hosts) == 1 {
		for _, host := range hosts {
			return host, nil
		}
	}

	// Look for bridge network
	if bridgeIP, ok := hosts["bridge"]; ok {
		return bridgeIP, nil
	}

	return "", fmt.Errorf("failed to resolve IP address for %s: network %q not found", ctx.GetServiceID(), tplVar)
}

func getPort(tplVar string, ctx TemplateContext) (string, error) {
	if ctx == nil {
		return "", errors.New("no context available for %%port%% resolution")
	}

	ports, err := ctx.GetPorts()
	if err != nil {
		return "", fmt.Errorf("failed to extract port list for %s: %w", ctx.GetServiceID(), err)
	}
	if len(ports) == 0 {
		return "", fmt.Errorf("no port found for %s", ctx.GetServiceID())
	}

	// No specific port requested, return last port
	if len(tplVar) == 0 {
		return strconv.Itoa(ports[len(ports)-1].Port), nil
	}

	// Try to parse as index
	idx, err := strconv.Atoi(tplVar)
	if err != nil {
		// Not an index, try to lookup by name
		for _, port := range ports {
			if port.Name == tplVar {
				return strconv.Itoa(port.Port), nil
			}
		}
		return "", fmt.Errorf("port %s not found for %s", tplVar, ctx.GetServiceID())
	}

	// Use as index
	if len(ports) <= idx {
		return "", fmt.Errorf("port index %d out of range for %s", idx, ctx.GetServiceID())
	}
	return strconv.Itoa(ports[idx].Port), nil
}

func getPid(_ string, ctx TemplateContext) (string, error) {
	if ctx == nil {
		return "", errors.New("no context available for %%pid%% resolution")
	}

	pid, err := ctx.GetPid()
	if err != nil {
		return "", fmt.Errorf("failed to get pid for %s: %w", ctx.GetServiceID(), err)
	}
	return strconv.Itoa(pid), nil
}

func getHostname(_ string, ctx TemplateContext) (string, error) {
	if ctx == nil {
		return "", errors.New("no context available for %%hostname%% resolution")
	}

	name, err := ctx.GetHostname()
	if err != nil {
		return "", fmt.Errorf("failed to get hostname for %s: %w", ctx.GetServiceID(), err)
	}
	return name, nil
}

func getExtraConfig(key string, ctx TemplateContext) (string, error) {
	if ctx == nil {
		return "", errors.New("no context available for %%kube_*%%/%%extra_*%% resolution")
	}

	value, err := ctx.GetExtraConfig(key)
	if err != nil {
		return "", fmt.Errorf("failed to get extra config %s for %s: %w", key, ctx.GetServiceID(), err)
	}
	return value, nil
}

func getEnvvar(envVar string, ctx TemplateContext) (string, error) {
	if len(envVar) == 0 {
		return "", errors.New("envvar name is missing")
	}

	if !allowEnvVar(envVar) {
		return "", fmt.Errorf("envvar %s is not allowed in template resolution", envVar)
	}

	value, found := os.LookupEnv(envVar)
	if !found {
		if ctx != nil {
			return "", fmt.Errorf("envvar %s not found for %s", envVar, ctx.GetServiceID())
		}
		return "", fmt.Errorf("envvar %s not found", envVar)
	}
	return value, nil
}

func allowEnvVar(envVar string) bool {
	if pkgconfigsetup.Datadog().GetBool("ad_disable_env_var_resolution") {
		return false
	}

	allowedEnvs := pkgconfigsetup.Datadog().GetStringSlice("ad_allowed_env_vars")

	// If the option is not set or is empty, all envs are allowed
	if len(allowedEnvs) == 0 {
		return true
	}

	return slices.ContainsFunc(allowedEnvs, func(env string) bool {
		return strings.EqualFold(env, envVar)
	})
}
