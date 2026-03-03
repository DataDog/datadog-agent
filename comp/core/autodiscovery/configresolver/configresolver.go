// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package configresolver resolves configuration templates against a given
// service by replacing template variables with corresponding data from the
// service.
package configresolver

import (
	"errors"
	"fmt"
	"sort"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	filter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/tmplvar"
)

// SubstituteTemplateEnvVars replaces %%ENV_VARIABLE%% from environment
// variables in the config init, instances, and logs config.
// When there is an error, it continues replacing. When there are multiple
// errors, the one returned is the one that happened first.
func SubstituteTemplateEnvVars(config *integration.Config) error {
	return substituteTemplateVariables(config, nil, nil)
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
		ServiceID:       svc.GetServiceID(),
		NodeName:        tpl.NodeName,
		Source:          tpl.Source,
		MetricsExcluded: svc.HasFilter(filter.MetricsFilter),
		LogsExcluded:    svc.HasFilter(filter.LogsFilter),
		ImageName:       svc.GetImageName(),
	}
	copy(resolvedConfig.InitConfig, tpl.InitConfig)
	copy(resolvedConfig.Instances, tpl.Instances)

	if namespace, err := svc.GetExtraConfig("namespace"); err == nil {
		resolvedConfig.PodNamespace = namespace
	}

	if resolvedConfig.IsCheckConfig() && !svc.IsReady() {
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

	if err := substituteTemplateVariables(&resolvedConfig, svc, postProcessor); err != nil {
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
func substituteTemplateVariables(config *integration.Config, svc listeners.Service, postProcessor func(interface{}) error) error {
	var err error
	for _, toResolve := range listDataToResolve(config) {
		var pp func(interface{}) error
		if toResolve.dtype == dataInstance {
			pp = postProcessor
		}
		resolver := tmplvar.NewTemplateResolver(toResolve.parser, pp, true)
		*toResolve.data, err = resolver.ResolveDataWithTemplateVars(*toResolve.data, svc)
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

type dataToResolve struct {
	data   *integration.Data
	dtype  dataType
	parser tmplvar.Parser
}

func listDataToResolve(config *integration.Config) []dataToResolve {
	res := []dataToResolve{
		{
			data:   &config.InitConfig,
			dtype:  dataInit,
			parser: tmplvar.YAMLParser,
		},
	}

	for i := 0; i < len(config.Instances); i++ {
		res = append(res, dataToResolve{
			data:   &config.Instances[i],
			dtype:  dataInstance,
			parser: tmplvar.YAMLParser,
		})
	}

	if config.IsLogConfig() {
		p := tmplvar.YAMLParser
		if config.Provider == names.Container || config.Provider == names.Kubernetes || config.Provider == names.KubeContainer {
			p = tmplvar.JSONParser
		}
		res = append(res, dataToResolve{
			data:   &config.LogsConfig,
			dtype:  dataLogs,
			parser: p,
		})
	}

	return res
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
