// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package configresolver resolves config templates using information from a
// service.
package configresolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	yaml "gopkg.in/yaml.v2"
)

type variableGetter func(ctx context.Context, key string, svc listeners.Service) (string, error)

var templateVariables = map[string]variableGetter{
	"host":     getHost,
	"pid":      getPid,
	"port":     getPort,
	"hostname": getHostname,
	"env":      getEnvvar,
	"extra":    getAdditionalTplVariables,
	"kube":     getAdditionalTplVariables,
}

// NoServiceError represents an error that indicates that there's a problem with a service
type NoServiceError struct {
	message string
}

// Error returns the error message
func (n *NoServiceError) Error() string {
	return n.message
}

// NewNoServiceError returns a new NoServiceError
func NewNoServiceError(message string) *NoServiceError {
	return &NoServiceError{
		message: message,
	}
}

// SubstituteTemplateEnvVars replaces %%ENV_VARIABLE%% from environment
// variables in the config init, instances, and logs config.
// When there is an error, it continues replacing. When there are multiple
// errors, the one returned is the one that happened first.
func SubstituteTemplateEnvVars(config *integration.Config) error {
	return substituteTemplateVariables(context.Background(), config, nil, nil)
}

// Resolve takes a template and a service and generates a config with
// valid connection info and relevant tags.
func Resolve(tpl integration.Config, svc listeners.Service) (integration.Config, error) {
	ctx := context.TODO()
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
		ServiceID:       svc.GetServiceID(),
		NodeName:        tpl.NodeName,
		Source:          tpl.Source,
		MetricsExcluded: svc.HasFilter(containers.MetricsFilter),
		LogsExcluded:    svc.HasFilter(containers.LogsFilter),
	}
	copy(resolvedConfig.InitConfig, tpl.InitConfig)
	copy(resolvedConfig.Instances, tpl.Instances)

	if resolvedConfig.IsCheckConfig() && !svc.IsReady(ctx) {
		return resolvedConfig, errors.New("unable to resolve, service not ready")
	}

	var tags []string
	var err error
	if tpl.CheckTagCardinality != "" {
		tags, err = svc.GetTagsWithCardinality(tpl.CheckTagCardinality)
	} else {
		tags, err = svc.GetTags()
	}
	if err != nil {
		return resolvedConfig, fmt.Errorf("couldn't get tags for service '%s', err: %w", svc.GetServiceID(), err)
	}

	var postProcessor func(interface{}) error

	if !tpl.IgnoreAutodiscoveryTags {
		postProcessor = tagsAdder(tags)
	}

	if err := substituteTemplateVariables(ctx, &resolvedConfig, svc, postProcessor); err != nil {
		return resolvedConfig, err
	}

	return resolvedConfig, nil
}

// substituteTemplateVariables replaces %%VARIABLES%% in the config init,
// instances, and logs config.
// When there is an error, it stops processing.
//
// Inside the `config` parameter, the `Instances` field holds strings representing a yaml document where the %%VARIABLES%% placeholders will be substituted.
// In order to do that, the string is decoded into a tree representing the yaml document.
// If not `nil`, the `postProcessor` function is invoked on that tree so that it can alter the yaml document and benefit from the yaml parsing.
// It can be used, for ex., to inject extra tags.
func substituteTemplateVariables(ctx context.Context, config *integration.Config, svc listeners.Service, postProcessor func(interface{}) error) error {
	var err error

	for _, toResolve := range listDataToResolve(config) {
		var pp func(interface{}) error
		if toResolve.dtype == dataInstance {
			pp = postProcessor
		}
		*toResolve.data, err = resolveDataWithTemplateVars(ctx, *toResolve.data, svc, toResolve.parser, pp)
		if err != nil {
			return err
		}
	}

	return nil
}

type dataType int

const (
	dataInit dataType = iota
	dataInstance
	dataLogs
)

type parser struct {
	marshal   func(interface{}) ([]byte, error)
	unmarshal func([]byte, interface{}) error
}

var jsonp = parser{
	marshal:   json.Marshal,
	unmarshal: json.Unmarshal,
}

var yamlp = parser{
	marshal:   yaml.Marshal,
	unmarshal: yaml.Unmarshal,
}

type dataToResolve struct {
	data   *integration.Data
	dtype  dataType
	parser parser
}

func listDataToResolve(config *integration.Config) []dataToResolve {
	res := []dataToResolve{
		{
			data:   &config.InitConfig,
			dtype:  dataInit,
			parser: yamlp,
		},
	}

	for i := 0; i < len(config.Instances); i++ {
		res = append(res, dataToResolve{
			data:   &config.Instances[i],
			dtype:  dataInstance,
			parser: yamlp,
		})
	}

	if config.IsLogConfig() {
		p := yamlp
		if config.Provider == names.Container || config.Provider == names.Kubernetes || config.Provider == names.KubeContainer {
			p = jsonp
		}
		res = append(res, dataToResolve{
			data:   &config.LogsConfig,
			dtype:  dataLogs,
			parser: p,
		})
	}

	return res
}

func resolveDataWithTemplateVars(ctx context.Context, data integration.Data, svc listeners.Service, parser parser, postProcessor func(interface{}) error) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var tree interface{}

	// Percent character is not allowed in unquoted yaml strings.
	data2 := strings.ReplaceAll(string(data), "%%", "‰")
	if err := parser.unmarshal([]byte(data2), &tree); err != nil {
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
			s, err := resolveStringWithTemplateVars(ctx, elem, svc)
			if err != nil {
				return data, err
			}
			// If a `‰` character hasn’t been consumed by a `%%var%%` template variable replacement,
			// let’s restore it to the initial `%%` value it had in the original string.
			if str, ok := s.(string); ok {
				s = strings.ReplaceAll(str, "‰", "%%")
			}
			top.set(s)

		case nil, int, bool:

		default:
			log.Errorf("Unknown type: %T", elem)
		}
	}

	if postProcessor != nil {
		if err := postProcessor(tree); err != nil {
			return data, err
		}
	}

	return parser.marshal(&tree)
}

// resolveStringWithTemplateVars takes a string as input and replaces all the `‰var_param‰` patterns by the value returned by the appropriate variable getter.
// It delegates all the work to resolveStringWithAdHocTemplateVars and implements only the following trick:
// for `‰host‰` patterns, if the value of the variable is an IPv6 *and* it appears in an URL context, then it is surrounded by square brackets.
// Indeed, IPv6 needs to be surrounded by square brackets inside URL to distinguish the colons of the IPv6 itself from the one separating the IP from the port
// like in: http://[::1]:80/
func resolveStringWithTemplateVars(ctx context.Context, in string, svc listeners.Service) (out interface{}, err error) {
	isThereAnIPv6Host := false

	adHocTemplateVars := make(map[string]variableGetter)
	for k, v := range templateVariables {
		if k == "host" {
			adHocTemplateVars[k] = func(ctx context.Context, tplVar string, svc listeners.Service) (string, error) {
				host, err := getHost(ctx, tplVar, svc)
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
	resolvedString, err := resolveStringWithAdHocTemplateVars(ctx, in, svc, adHocTemplateVars)
	if err != nil {
		return resolvedString, err
	}

	if !isThereAnIPv6Host {
		return resolvedString, err
	}

	if _, isString := resolvedString.(string); !isString {
		return resolvedString, err
	}

	adHocTemplateVars = map[string]variableGetter{
		"host": func(_ context.Context, _ string, _ listeners.Service) (string, error) {
			return "127.0.0.1", nil
		},
	}
	resolvedStringWithFakeIPv4, err := resolveStringWithAdHocTemplateVars(ctx, resolvedString.(string), svc, adHocTemplateVars)
	if err != nil {
		return resolvedString, err
	}

	_, err = url.Parse(resolvedStringWithFakeIPv4.(string))
	if err != nil {
		return resolvedString, nil
	}

	adHocTemplateVars = map[string]variableGetter{
		"host": func(ctx context.Context, tplVar string, svc listeners.Service) (string, error) {
			host, err := getHost(ctx, tplVar, svc)
			var sb strings.Builder
			sb.WriteByte('[')
			sb.WriteString(host)
			sb.WriteByte(']')
			return sb.String(), err
		},
	}
	resolvedStringWithIPv6, err := resolveStringWithAdHocTemplateVars(ctx, resolvedString.(string), svc, adHocTemplateVars)
	if err != nil {
		return resolvedString, err
	}

	_, err = url.Parse(resolvedStringWithIPv6.(string))
	if err != nil {
		return resolveStringWithAdHocTemplateVars(ctx, in, svc, templateVariables)
	}

	return resolvedStringWithIPv6, err
}

var varPattern = regexp.MustCompile(`‰(.+?)(?:_(.+?))?‰`)

// resolveStringWithAdHocTemplateVars takes a string as input and replaces all the `‰var_param‰` patterns by the value returned by the appropriate variable getter.
// The variable getters are passed as last parameter.
// If the input string is composed of *only* a `‰var_param‰` pattern and the result of the substitution is a boolean or a number, then the function returns a boolean or a number instead of a string.
func resolveStringWithAdHocTemplateVars(ctx context.Context, in string, svc listeners.Service, templateVariables map[string]variableGetter) (out interface{}, err error) {
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
			resolvedVar, e := f(ctx, varKey, svc)
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
			if svc != nil {
				err = fmt.Errorf("unable to add tags for service '%s', err: %w", svc.GetServiceID(), err)
			}
			return out, err
		}
	}
	sb.WriteString(in[varIndexes[len(varIndexes)-1][1]:])

	out = sb.String()

	if len(varIndexes) == 1 &&
		varIndexes[0][0] == 0 &&
		varIndexes[0][1] == len(in) {

		if i, e := strconv.ParseInt(out.(string), 0, 64); e == nil {
			return i, err
		}
		if b, e := strconv.ParseBool(out.(string)); e == nil {
			return b, err
		}
	}

	return
}

func tagsAdder(tags []string) func(interface{}) error {
	return func(tree interface{}) error {
		if len(tags) == 0 {
			return nil
		}

		if typedTree, ok := tree.(map[interface{}]interface{}); ok {
			// Use a set to remove duplicates
			tagSet := make(map[string]struct{})
			if typedTreeTags, ok := typedTree["tags"]; ok {
				if tagList, ok := typedTreeTags.([]interface{}); !ok {
					log.Errorf("Wrong type for `tags` in config. Expected []interface{}, got %T", typedTree["tags"])
				} else {
					for _, tag := range tagList {
						if t, ok := tag.(string); !ok {
							log.Errorf("Wrong type for tag \"%#v\". Expected string, got %T", tag, tag)
						} else {
							tagSet[t] = struct{}{}
						}
					}
				}
			}

			for _, t := range tags {
				tagSet[t] = struct{}{}
			}

			allTags := make([]string, len(tagSet))

			i := 0
			for k := range tagSet {
				allTags[i] = k
				i++
			}

			sort.Strings(allTags)

			typedTree["tags"] = allTags
		}
		return nil
	}
}

func getHost(ctx context.Context, tplVar string, svc listeners.Service) (string, error) {
	if svc == nil {
		return "", NewNoServiceError("No service. %%%%host%%%% is not allowed")
	}

	hosts, err := svc.GetHosts(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to extract IP address for container %s, ignoring it. Source error: %s", svc.GetServiceID(), err)
	}
	if len(hosts) == 0 {
		return "", fmt.Errorf("no network found for container %s, ignoring it", svc.GetServiceID())
	}

	// a network was specified
	if ip, ok := hosts[tplVar]; ok {
		return ip, nil
	}
	log.Debugf("Network %q not found, trying bridge IP instead", tplVar)

	// otherwise use fallback policy
	ip, err := getFallbackHost(hosts)
	if err != nil {
		return "", fmt.Errorf("failed to resolve IP address for container %s, ignoring it. Source error: %s", svc.GetServiceID(), err)
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

// getPort returns ports of the service
func getPort(ctx context.Context, tplVar string, svc listeners.Service) (string, error) {
	if svc == nil {
		return "", NewNoServiceError("No service. %%%%port%%%% is not allowed")
	}

	ports, err := svc.GetPorts(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to extract port list for container %s, ignoring it. Source error: %s", svc.GetServiceID(), err)
	} else if len(ports) == 0 {
		return "", fmt.Errorf("no port found for container %s - ignoring it", svc.GetServiceID())
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
		return "", fmt.Errorf("port %s not found, skipping container %s", tplVar, svc.GetServiceID())
	}
	if len(ports) <= idx {
		return "", fmt.Errorf("index given for the port template var is too big, skipping container %s", svc.GetServiceID())
	}
	return strconv.Itoa(ports[idx].Port), nil
}

// getPid returns the process identifier of the service
func getPid(ctx context.Context, _ string, svc listeners.Service) (string, error) {
	if svc == nil {
		return "", NewNoServiceError("No service. %%%%pid%%%% is not allowed")
	}

	pid, err := svc.GetPid(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get pid for service %s, skipping config - %s", svc.GetServiceID(), err)
	}
	return strconv.Itoa(pid), nil
}

// getHostname returns the hostname of the service, to be used
// when the IP is unavailable or erroneous
func getHostname(ctx context.Context, _ string, svc listeners.Service) (string, error) {
	if svc == nil {
		return "", NewNoServiceError("No service. %%%%hostname%%%% is not allowed")
	}

	name, err := svc.GetHostname(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get hostname for service %s, skipping config - %s", svc.GetServiceID(), err)
	}
	return name, nil
}

// getAdditionalTplVariables returns listener-specific template variables.
// It resolves template variables prefixed with kube_ or extra_
// Even though it gets the data from the same listener method GetExtraConfig, the kube_ and extra_
// prefixes are customer facing, we support both of them for a better user experience depending on
// the AD listener and what the template variable represents.
func getAdditionalTplVariables(_ context.Context, tplVar string, svc listeners.Service) (string, error) {
	if svc == nil {
		return "", NewNoServiceError("No service. %%%%extra_*%%%% or %%%%kube_*%%%% are not allowed")
	}

	value, err := svc.GetExtraConfig(tplVar)
	if err != nil {
		return "", fmt.Errorf("failed to get extra info for service %s, skipping config - %s", svc.GetServiceID(), err)
	}
	return value, nil
}

// getEnvvar returns a system environment variable if found
func getEnvvar(_ context.Context, envVar string, svc listeners.Service) (string, error) {
	if len(envVar) == 0 {
		if svc != nil {
			return "", fmt.Errorf("envvar name is missing, skipping service %s", svc.GetServiceID())
		}
		return "", fmt.Errorf("envvar name is missing")
	}
	value, found := os.LookupEnv(envVar)
	if !found {
		if svc != nil {
			return "", fmt.Errorf("failed to retrieve envvar %s, skipping service %s", envVar, svc.GetServiceID())
		}
		return "", fmt.Errorf("failed to retrieve envvar %s", envVar)
	}
	return value, nil
}
