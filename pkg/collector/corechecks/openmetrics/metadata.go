// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openmetrics

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	collectorcheck "github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var semverPattern = regexp.MustCompile(`v?(?P<major>0|[1-9][0-9]*)\.(?P<minor>0|[1-9][0-9]*)\.(?P<patch>0|[1-9][0-9]*)(?:-(?P<release>(?:0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+(?P<build>[0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?`)

func submitMetadata(metricName string, label string, options map[string]interface{}, checkID string) transformerFunc {
	return func(_ parsedMetric, samples []sampleDatum, _ runtimeData, _ sender.Sender) {
		for _, sample := range samples {
			value, ok := sample.sample.Labels[label]
			if !ok {
				continue
			}
			setMetadata(checkID, metricName, value, options)
		}
	}
}

func submitLegacyMetadata(labelMap map[string]string, checkID string) transformerFunc {
	return func(_ parsedMetric, samples []sampleDatum, _ runtimeData, _ sender.Sender) {
		if len(samples) == 0 {
			return
		}
		labels := samples[0].sample.Labels
		for metadataName, labelName := range labelMap {
			value, ok := labels[labelName]
			if ok {
				setMetadata(checkID, metadataName, value, nil)
			}
		}
	}
}

func compileMetadataLabelMap(raw interface{}) (map[string]string, error) {
	rawMap, ok := normalizeMap(raw)
	if !ok {
		return nil, errors.New("the `label_map` parameter must be a mapping")
	}
	out := make(map[string]string, len(rawMap))
	for metadataName, rawLabelName := range rawMap {
		labelName, ok := rawLabelName.(string)
		if !ok {
			return nil, fmt.Errorf("value for metadata `%s` of parameter `label_map` must be a string", metadataName)
		}
		out[metadataName] = labelName
	}
	return out, nil
}

func setMetadata(checkID string, name string, value string, options map[string]interface{}) {
	inv, err := collectorcheck.GetInventoryChecksContext()
	if err != nil {
		return
	}

	if name != "version" {
		inv.Set(checkID, name, value)
		return
	}

	versionData, err := transformVersion(value, options)
	if err != nil {
		log.Debugf("Unable to transform `%s` metadata value `%s`: %s", name, value, err)
		return
	}
	for key, metadataValue := range versionData {
		inv.Set(checkID, key, metadataValue)
	}
}

func transformVersion(version string, options map[string]interface{}) (map[string]string, error) {
	scheme, _ := options["scheme"].(string)
	if scheme == "" {
		scheme = "semver"
	}

	var parts map[string]string
	var err error
	switch scheme {
	case "semver":
		parts, err = regexVersionParts(semverPattern, version)
	case "regex":
		pattern, _ := options["pattern"].(string)
		if pattern == "" {
			return nil, errors.New("version scheme `regex` requires a `pattern` option")
		}
		compiled, compileErr := regexp.Compile(pattern)
		if compileErr != nil {
			return nil, compileErr
		}
		parts, err = regexVersionParts(compiled, version)
	case "parts":
		rawPartMap, ok := normalizeMap(options["part_map"])
		if !ok || len(rawPartMap) == 0 {
			return nil, errors.New("version scheme `parts` requires a `part_map` option")
		}
		parts = map[string]string{}
		for partName, partValue := range rawPartMap {
			if partValue != nil {
				parts[partName] = fmt.Sprint(partValue)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported version scheme `%s`", scheme)
	}
	if err != nil {
		return nil, err
	}

	finalScheme := scheme
	if scheme == "regex" || scheme == "parts" {
		if configuredFinalScheme, _ := options["final_scheme"].(string); configuredFinalScheme != "" {
			finalScheme = configuredFinalScheme
		} else {
			finalScheme = CheckName
		}
	}

	out := map[string]string{
		"version.raw":    version,
		"version.scheme": finalScheme,
	}
	for partName, partValue := range parts {
		out["version."+partName] = partValue
	}
	return out, nil
}

func regexVersionParts(pattern *regexp.Regexp, version string) (map[string]string, error) {
	match := pattern.FindStringSubmatch(version)
	if match == nil {
		return nil, errors.New("version does not match the regular expression pattern")
	}

	parts := map[string]string{}
	for i, name := range pattern.SubexpNames() {
		if i == 0 || name == "" || match[i] == "" {
			continue
		}
		parts[name] = match[i]
	}
	if len(parts) == 0 {
		return nil, errors.New("regular expression pattern has no named subgroups")
	}
	return parts, nil
}
