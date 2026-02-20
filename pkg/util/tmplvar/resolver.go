// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tmplvar provides template variable resolution utilities that can be
// shared across components (autodiscovery, tagger, etc.)
package tmplvar

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"gopkg.in/yaml.v2"
)

type Resolvable interface {
	// GetServiceID returns a unique identifier for this service (for error messages)
	GetServiceID() string

	// GetHosts returns a map of network name to IP address
	GetHosts() (map[string]string, error)

	// GetPorts returns the list of exposed ports
	GetPorts() ([]workloadmeta.ContainerPort, error)

	// GetPid returns the process ID
	GetPid() (int, error)

	// GetHostname returns the hostname
	GetHostname() (string, error)

	// GetExtraConfig returns listener-specific configuration (for %%kube_*%% and %%extra_*%%)
	GetExtraConfig(key string) (string, error)
}

// NoResolverError represents an error that indicates that there's a problem with a service
type NoResolverError struct {
	message string
}

// Error returns the error message
func (n *NoResolverError) Error() string {
	return n.message
}

// noResolverError returns a new NoResolverError
func noResolverError(message string) *NoResolverError {
	return &NoResolverError{
		message: message,
	}
}

// VariableGetter is a function that resolves a template variable
type VariableGetter func(key string, res Resolvable) (string, error)

// Parser handles marshaling/unmarshaling of data
type Parser struct {
	Marshal   func(interface{}) ([]byte, error)
	Unmarshal func([]byte, interface{}) error
}

// JSONParser is a parser for JSON data
var JSONParser = Parser{
	Marshal:   json.Marshal,
	Unmarshal: json.Unmarshal,
}

// YAMLParser is a parser for YAML data
var YAMLParser = Parser{
	Marshal:   yaml.Marshal,
	Unmarshal: yaml.Unmarshal,
}

var varPattern = regexp.MustCompile(`‰(.+?)(?:_(.+?))?‰`)

type TemplateResolver struct {
	parser        Parser
	postProcessor func(interface{}) error
	supportedVars map[string]VariableGetter
}

func NewTemplateResolver(parser Parser, postProcessor func(interface{}) error, supportEnvVars bool) *TemplateResolver {
	templateVariables := map[string]VariableGetter{
		"host":     GetHost,
		"pid":      GetPid,
		"port":     GetPort,
		"hostname": GetHostname,
		"extra":    GetAdditionalTplVariables,
		"kube":     GetAdditionalTplVariables,
	}
	if supportEnvVars {
		templateVariables["env"] = GetEnvvar
	}

	return &TemplateResolver{parser: parser, postProcessor: postProcessor, supportedVars: templateVariables}
}

// ResolveDataWithTemplateVars resolves template variables in a data structure (YAML/JSON).
// It walks through the tree structure and replaces %%var%% patterns in all strings.
// If postProcessor is not nil, it's called on the tree before marshaling back.
func (t TemplateResolver) ResolveDataWithTemplateVars(data []byte, res Resolvable) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var tree interface{}

	// Percent character is not allowed in unquoted yaml strings.
	data2 := strings.ReplaceAll(string(data), "%%", "‰")
	if err := t.parser.Unmarshal([]byte(data2), &tree); err != nil {
		return data, err
	}

	type treePointer struct {
		get func() interface{}
		set func(interface{})
	}

	stack := []treePointer{
		{
			get: func() interface{} {
				return tree
			},
			set: func(x interface{}) {
				tree = x
			},
		},
	}

	for len(stack) > 0 {
		n := len(stack) - 1
		top := stack[n]
		stack = stack[:n]

		switch elem := top.get().(type) {

		case map[interface{}]interface{}:
			for k, v := range elem {
				k2, v2 := k, v
				stack = append(stack, treePointer{
					get: func() interface{} {
						return v2
					},
					set: func(x interface{}) {
						elem[k2] = x
					},
				})
			}

		case map[string]interface{}:
			for k, v := range elem {
				k2, v2 := k, v
				stack = append(stack, treePointer{
					get: func() interface{} {
						return v2
					},
					set: func(x interface{}) {
						elem[k2] = x
					},
				})
			}

		case []interface{}:
			for i, v := range elem {
				i2, v2 := i, v
				stack = append(stack, treePointer{
					get: func() interface{} {
						return v2
					},
					set: func(x interface{}) {
						elem[i2] = x
					},
				})
			}

		case string:
			s, err := resolveStringWithTemplateVars(elem, res, t.supportedVars)
			if err != nil {
				return data, err
			}
			// If a `‰` character hasn't been consumed by a `%%var%%` template variable replacement,
			// let's restore it to the initial `%%` value it had in the original string.
			if str, ok := s.(string); ok {
				s = strings.ReplaceAll(str, "‰", "%%")
			}
			top.set(s)

		case nil, int, bool:

		default:
			log.Errorf("Unknown type: %T", elem)
		}
	}

	if t.postProcessor != nil {
		if err := t.postProcessor(tree); err != nil {
			return data, err
		}
	}

	return t.parser.Marshal(&tree)
}

// resolveStringWithTemplateVars takes a string as input and replaces all the `‰var_param‰` patterns by the value returned by the appropriate variable getter.
// It delegates all the work to resolveStringWithAdHocTemplateVars and implements only the following trick:
// for `‰host‰` patterns, if the value of the variable is an IPv6 *and* it appears in an URL context, then it is surrounded by square brackets.
// Indeed, IPv6 needs to be surrounded by square brackets inside URL to distinguish the colons of the IPv6 itself from the one separating the IP from the port
// like in: http://[::1]:80/
func resolveStringWithTemplateVars(in string, res Resolvable, templateVars map[string]VariableGetter) (out interface{}, err error) {
	isThereAnIPv6Host := false

	adHocTemplateVars := make(map[string]VariableGetter)
	for k, v := range templateVars {
		if k == "host" {
			adHocTemplateVars[k] = func(tplVar string, res Resolvable) (string, error) {
				host, err := v(tplVar, res)
				if apiutil.IsIPv6(host) {
					isThereAnIPv6Host = true
					if tplVar != "" {
						return fmt.Sprintf("‰host_%s‰", tplVar), nil
					}
					return "‰host‰", nil
				}
				return host, err
			}
		} else {
			adHocTemplateVars[k] = v
		}
	}
	resolvedString, err := resolveStringWithAdHocTemplateVars(in, res, adHocTemplateVars)
	if err != nil {
		return resolvedString, err
	}

	if !isThereAnIPv6Host {
		return resolvedString, err
	}

	if _, isString := resolvedString.(string); !isString {
		return resolvedString, err
	}

	adHocTemplateVars = map[string]VariableGetter{
		"host": func(_ string, _ Resolvable) (string, error) {
			return "127.0.0.1", nil
		},
	}
	resolvedStringWithFakeIPv4, err := resolveStringWithAdHocTemplateVars(resolvedString.(string), res, adHocTemplateVars)
	if err != nil {
		return resolvedString, err
	}

	_, err = url.Parse(resolvedStringWithFakeIPv4.(string))
	if err != nil {
		return resolvedString, nil
	}

	adHocTemplateVars = map[string]VariableGetter{
		"host": func(tplVar string, res Resolvable) (string, error) {
			host, err := GetHost(tplVar, res)
			var sb strings.Builder
			sb.WriteByte('[')
			sb.WriteString(host)
			sb.WriteByte(']')
			return sb.String(), err
		},
	}
	resolvedStringWithIPv6, err := resolveStringWithAdHocTemplateVars(resolvedString.(string), res, adHocTemplateVars)
	if err != nil {
		return resolvedString, err
	}

	_, err = url.Parse(resolvedStringWithIPv6.(string))
	if err != nil {
		return resolveStringWithAdHocTemplateVars(in, res, templateVars)
	}

	return resolvedStringWithIPv6, err
}

// resolveStringWithAdHocTemplateVars takes a string as input and replaces all the `‰var_param‰` patterns by the value returned by the appropriate variable getter.
// The variable getters are passed as last parameter.
// If the input string is composed of *only* a `‰var_param‰` pattern and the result of the substitution is a boolean or a number, then the function returns a boolean or a number instead of a string.
func resolveStringWithAdHocTemplateVars(in string, res Resolvable, templateVariables map[string]VariableGetter) (out interface{}, err error) {
	varIndexes := varPattern.FindAllStringSubmatchIndex(in, -1)

	if len(varIndexes) == 0 {
		return in, nil
	}

	var sb strings.Builder

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

		if f, found := templateVariables[varName]; found {
			resolvedVar, e := f(varKey, res)
			if e != nil {
				err = e
			}
			sb.WriteString(resolvedVar)
		} else {
			endTagIdx := varIndexes[i][5]
			if endTagIdx == -1 {
				endTagIdx = varIndexes[i][3]
			}
			err := fmt.Errorf("invalid %%%%%s%%%% tag", in[varIndexes[i][2]:endTagIdx])
			if res != nil {
				err = fmt.Errorf("unable to add tags for service '%v', err: %w", res.GetServiceID(), err)
			}
			return out, err
		}
	}
	sb.WriteString(in[varIndexes[len(varIndexes)-1][1]:])

	out = sb.String()

	if len(varIndexes) == 1 &&
		varIndexes[0][0] == 0 &&
		varIndexes[0][1] == len(in) {

		// %%env_*%% values should not be coerced as they may mismatch with checks
		// or be parsed incorrectly if they have a base like base-0 ex: "0123456" becomes 42798
		singleVarName := in[varIndexes[0][2]:varIndexes[0][3]]
		if singleVarName != "env" {
			if i, e := strconv.ParseInt(out.(string), 0, 64); e == nil {
				return i, err
			}
			if b, e := strconv.ParseBool(out.(string)); e == nil {
				return b, err
			}
		}
	}
	return
}

// GetHost resolves the %%host%% template variable
func GetHost(tplVar string, res Resolvable) (string, error) {
	if res == nil {
		return "", noResolverError("no resolver. %%%%host%%%% is not allowed")
	}

	hosts, err := res.GetHosts()
	if err != nil {
		return "", fmt.Errorf("failed to extract IP address for container %s, ignoring it. Source error: %s", res.GetServiceID(), err)
	}
	if len(hosts) == 0 {
		return "", fmt.Errorf("no network found for container %s, ignoring it", res.GetServiceID())
	}

	// a network was specified
	if ip, ok := hosts[tplVar]; ok {
		return ip, nil
	}
	log.Debugf("Network %q not found, trying bridge IP instead", tplVar)

	// otherwise use fallback policy
	ip, err := getFallbackHost(hosts)
	if err != nil {
		return "", fmt.Errorf("failed to resolve IP address for container %s, ignoring it. Source error: %s", res.GetServiceID(), err)
	}

	return ip, nil
}

// getFallbackHost implements the fallback strategy to get a service's IP address
// the current strategy is:
//   - if there's only one network we use its IP
//   - otherwise we look for the bridge net and return its IP address
//   - if we can't find it we fail because we shouldn't try and guess the IP address
func getFallbackHost(hosts map[string]string) (string, error) {
	if len(hosts) == 1 {
		for _, host := range hosts {
			return host, nil
		}
	}

	bridgeIP, bridgeIsPresent := hosts["bridge"]
	if bridgeIsPresent {
		return bridgeIP, nil
	}

	return "", errors.New("not able to determine which network is reachable")
}

// GetPort resolves the %%port%% template variable
func GetPort(tplVar string, res Resolvable) (string, error) {
	if res == nil {
		return "", noResolverError("no resolver. %%%%host%%%% is not allowed")
	}

	ports, err := res.GetPorts()
	if err != nil {
		return "", fmt.Errorf("failed to extract port list for container %s, ignoring it. Source error: %s", res.GetServiceID(), err)
	} else if len(ports) == 0 {
		return "", fmt.Errorf("no port found for container %s - ignoring it", res.GetServiceID())
	}

	if len(tplVar) == 0 {
		return strconv.Itoa(ports[len(ports)-1].Port), nil
	}

	idx, err := strconv.Atoi(tplVar)
	if err != nil {
		// The template variable is not an index so try to lookup port by name.
		for _, port := range ports {
			if port.Name == tplVar {
				return strconv.Itoa(port.Port), nil
			}
		}
		return "", fmt.Errorf("port %s not found, skipping container %s", tplVar, res.GetServiceID())
	}
	if len(ports) <= idx {
		return "", fmt.Errorf("index given for the port template var is too big, skipping container %s", res.GetServiceID())
	}
	return strconv.Itoa(ports[idx].Port), nil
}

// GetPid resolves the %%pid%% template variable
func GetPid(_ string, res Resolvable) (string, error) {
	if res == nil {
		return "", noResolverError("no resolver. %%%%pid%%%% is not allowed")
	}

	pid, err := res.GetPid()
	if err != nil {
		return "", fmt.Errorf("failed to get pid for service %s, skipping config - %s", res.GetServiceID(), err)
	}
	return strconv.Itoa(pid), nil
}

// GetHostname resolves the %%hostname%% template variable
func GetHostname(_ string, res Resolvable) (string, error) {
	if res == nil {
		return "", noResolverError("no resolver. %%%%hostname%%%% is not allowed")
	}

	name, err := res.GetHostname()
	if err != nil {
		return "", fmt.Errorf("failed to get hostname for service %s, skipping config - %s", res.GetServiceID(), err)
	}
	return name, nil
}

// GetAdditionalTplVariables resolves listener-specific template variables (%%kube_*%% and %%extra_*%%)
func GetAdditionalTplVariables(tplVar string, res Resolvable) (string, error) {
	if res == nil {
		return "", noResolverError("no resolver. %%%%extra_*%%%% or %%%%kube_*%%%% are not allowed")
	}

	value, err := res.GetExtraConfig(tplVar)
	if err != nil {
		return "", fmt.Errorf("failed to get extra info for service %s, skipping config - %s", res.GetServiceID(), err)
	}
	return value, nil
}

// GetEnvvar resolves the %%env_*%% template variable
func GetEnvvar(envVar string, res Resolvable) (string, error) {
	if len(envVar) == 0 {
		if res != nil {
			return "", fmt.Errorf("envvar name is missing, skipping service %s", res.GetServiceID())
		}
		return "", errors.New("envvar name is missing")
	}

	if !allowEnvVar(envVar) {
		if res != nil {
			return "", fmt.Errorf("envvar %s is not allowed in check configs, skipping service %s", envVar, res.GetServiceID())
		}
		return "", fmt.Errorf("envvar %s is not allowed in check configs", envVar)
	}

	value, found := os.LookupEnv(envVar)
	if !found {
		if res != nil {
			return "", fmt.Errorf("failed to retrieve envvar %s, skipping service %s", envVar, res.GetServiceID())
		}
		return "", fmt.Errorf("failed to retrieve envvar %s", envVar)
	}
	return value, nil
}

func allowEnvVar(envVar string) bool {
	if pkgconfigsetup.Datadog().GetBool("ad_disable_env_var_resolution") {
		return false
	}

	allowedEnvs := pkgconfigsetup.Datadog().GetStringSlice("ad_allowed_env_vars")

	// If the option is not set or is empty, the default behavior applies: all
	// envs are allowed.
	if len(allowedEnvs) == 0 {
		return true
	}

	return slices.ContainsFunc(allowedEnvs, func(env string) bool {
		return strings.EqualFold(env, envVar)
	})
}
