// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package collectors

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/tmplvar"
)

var templateVariables = map[string]struct{}{
	"label": {},
}

// retrieveMappingFromConfig gets a stringmapstring config key and
// lowercases all map keys to make envvar and yaml sources consistent
func retrieveMappingFromConfig(configKey string) map[string]string {
	labelsList := config.Datadog.GetStringMapString(configKey)
	for label, value := range labelsList {
		delete(labelsList, label)
		labelsList[strings.ToLower(label)] = value
	}

	return labelsList
}

func resolveTag(tmpl, label string) string {
	vars := tmplvar.ParseString(tmpl)
	tagName := tmpl
	for _, v := range vars {
		if _, ok := templateVariables[string(v.Name)]; ok {
			tagName = strings.Replace(tagName, string(v.Raw), label, -1)
			continue
		}
		tagName = strings.Replace(tagName, string(v.Raw), "", -1)
	}
	return tagName
}
