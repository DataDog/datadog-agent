// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"text/template"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/util/jsonquery"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/Masterminds/sprig"
	"gopkg.in/yaml.v3"
)

// getter applies jq query to get string value from json or yaml raw data
type getter func([]byte, string) (string, error)

// readContent unmarshal file
func readContent(filePath string) (interface{}, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return "", err
	}

	var content interface{}
	if err := json.Unmarshal(data, &content); err != nil {
		if err := yaml.Unmarshal(data, &content); err != nil {
			return string(data), err
		}
	}

	return content, nil
}

// jsonGetter retrieves a property from a JSON file (jq style syntax)
func jsonGetter(data []byte, query string) (string, error) {
	var jsonContent interface{}
	if err := json.Unmarshal(data, &jsonContent); err != nil {
		return "", err
	}
	value, _, err := jsonquery.RunSingleOutput(query, jsonContent)
	return value, err
}

// yamlGetter retrieves a property from a YAML file (jq style syntax)
func yamlGetter(data []byte, query string) (string, error) {
	var yamlContent interface{}
	if err := yaml.Unmarshal(data, &yamlContent); err != nil {
		return "", err
	}
	yamlContent = jsonquery.NormalizeYAMLForGoJQ(yamlContent)
	value, _, err := jsonquery.RunSingleOutput(query, yamlContent)
	return value, err
}

// regexpGetter retrieves the leftmost property matching regexp
func regexpGetter(data []byte, expr string) (string, error) {
	re, err := regexp.Compile(expr)
	if err != nil {
		return "", err
	}

	match := re.Find(data)
	if match == nil {
		return "", nil
	}

	return string(match), nil
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
func instanceToEventData(instance eval.Instance, allowedFields []string) event.Data {
	data := event.Data{}

	for k, v := range instance.Vars() {
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
func instanceToReport(instance resolvedInstance, passed bool, allowedFields []string) *compliance.Report {
	var data event.Data
	var resourceReport compliance.ReportResource

	if instance != nil {
		data = instanceToEventData(instance, allowedFields)
		resourceReport = compliance.ReportResource{
			ID:   instance.ID(),
			Type: instance.Type(),
		}
	}

	return &compliance.Report{
		Resource: resourceReport,
		Passed:   passed,
		Data:     data,
	}
}

// instanceToReports converts an evaluated instanceResult to reports
// filtering out fields not on the allowedFields list
func instanceResultToReport(result *eval.InstanceResult, allowedFields []string) *compliance.Report {
	resolvedInstance, _ := result.Instance.(resolvedInstance)
	return instanceToReport(resolvedInstance, result.Passed, allowedFields)
}
