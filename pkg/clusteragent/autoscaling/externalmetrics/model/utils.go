// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/tmplvar"
)

func parseDatadogMetricValue(s string) (float64, error) {
	if len(s) == 0 {
		return 0, nil
	}

	return strconv.ParseFloat(s, 64)
}

func formatDatadogMetricValue(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

type tagGetter func(context.Context) (string, error)

var templatedTags = map[string]tagGetter{
	"kube_cluster_name": func(ctx context.Context) (string, error) {
		hname, err := hostname.Get(ctx)
		if err != nil {
			return "", err
		}
		return clustername.GetClusterNameTagValue(ctx, hname), nil
	},
}

// resolveQuery replaces the template variables in the query
// The supported template variable types are %%tag_<tag_name>%% and %%env_<ENV_VAR>%%
// The only supported <tag_name> in %%tag_<tag_name>%% is kube_cluster_name
func resolveQuery(q string) (string, error) {
	vars := tmplvar.ParseString(q)
	if len(vars) == 0 {
		return "", nil
	}

	result := q
	for _, tplVar := range vars {
		switch string(tplVar.Name) {
		case "tag":
			tagGetter, found := templatedTags[string(tplVar.Key)]
			if !found {
				return "", fmt.Errorf("cannot resolve tag template %q: tag is not supported", tplVar.Key)
			}
			tagVal, err := tagGetter(context.TODO())
			if err != nil {
				return "", fmt.Errorf("cannot resolve tag template %q: %w", tplVar.Key, err)
			}
			result = strings.ReplaceAll(result, string(tplVar.Raw), tagVal)
		case "env":
			envVal, found := os.LookupEnv(string(tplVar.Key))
			if !found {
				return "", fmt.Errorf("failed to retrieve env var %q", tplVar.Key)
			}
			result = strings.ReplaceAll(result, string(tplVar.Raw), envVal)
		default:
			return "", fmt.Errorf("template variable %q is unknown", tplVar.Name)
		}
	}

	return result, nil
}
