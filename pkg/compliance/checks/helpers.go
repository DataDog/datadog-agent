// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"text/template"

	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/util/jsonquery"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/Masterminds/sprig"
	"gopkg.in/yaml.v2"
)

// getter applies jq query to get string value from json or yaml raw data
type getter func([]byte, string) (string, error)

// jsonGetter retrieves a property from a JSON file (jq style syntax)
func jsonGetter(data []byte, query string) (string, error) {
	var jsonContent interface{}
	if err := json.Unmarshal(data, &jsonContent); err != nil {
		return "", err
	}
	value, _, err := jsonquery.RunSingleOutput(query, jsonContent)
	return value, err
}

// jsonGetter retrieves a property from a YAML file (jq style syntax)
func yamlGetter(data []byte, query string) (string, error) {
	var yamlContent map[string]interface{}
	if err := yaml.Unmarshal(data, &yamlContent); err != nil {
		return "", err
	}
	value, _, err := jsonquery.RunSingleOutput(query, yamlContent)
	return value, err
}

// queryValueFromFile retrieves a value from a file with the provided getter func
func queryValueFromFile(filePath string, query string, get getter) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return "", err
	}

	return get(data, query)
}

// evalGoTemplate evaluates a go-style template on an object
func evalGoTemplate(s string, obj interface{}) string {
	tmpl, err := template.New("tmpl").Funcs(sprig.TxtFuncMap()).Parse(s)
	if err != nil {
		log.Warnf("failed to parse template %q: %v", s, err)
		return ""
	}

	b := &strings.Builder{}
	if err := tmpl.Execute(b, obj); err != nil {
		log.Tracef("failed to execute template %q: %v", s, err)
		return ""
	}
	return b.String()
}

// wrapErrorWithID wraps an error with an ID (e.g. rule ID)
func wrapErrorWithID(id string, err error) error {
	return fmt.Errorf("%s: %w", id, err)
}

// instanceToEventData converts an instance to event data filtering out fields not on the allowedFields list
func instanceToEventData(instance *eval.Instance, allowedFields []string) event.Data {
	data := event.Data{}

	for k, v := range instance.Vars {
		allow := false
		for _, a := range allowedFields {
			if k == a {
				allow = true
				break
			}
		}
		if !allow {
			continue
		}
		data[k] = v
	}
	return data
}

// instanceToReport converts an instance and passed status to report
// filtering out fields not on the allowedFields list
func instanceToReport(instance *eval.Instance, passed bool, allowedFields []string) *report {
	var data event.Data

	if instance != nil {
		data = instanceToEventData(instance, allowedFields)
	}

	return &report{
		passed: passed,
		data:   data,
	}
}

// instanceToReport converts an evaluated instanceResult to report
// filtering out fields not on the allowedFields list
func instanceResultToReport(result *eval.InstanceResult, allowedFields []string) *report {
	return instanceToReport(result.Instance, result.Passed, allowedFields)
}
