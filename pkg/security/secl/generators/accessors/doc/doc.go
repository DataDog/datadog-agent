//go:generate go run github.com/mailru/easyjson/easyjson $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package doc

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/generators/accessors/common"
)

// easyjson:json
type documentation struct {
	Types []eventType `json:"secl"`
}

// easyjson:json
type eventType struct {
	Name             string              `json:"name"`
	Definition       string              `json:"definition"`
	Type             string              `json:"type"`
	FromAgentVersion string              `json:"from_agent_version"`
	Properties       []eventTypeProperty `json:"properties"`
}

// easyjson:json
type eventTypeProperty struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Doc  string `json:"definition"`
}

func prettyprint(v interface{}) ([]byte, error) {
	base, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	if err := json.Indent(&out, base, "", "  "); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

// GenerateDocJSON generates the SECL json documentation file to the provided outputPath
func GenerateDocJSON(module *common.Module, outputPath string) error {
	kinds := make(map[string][]eventTypeProperty)

	for name, field := range module.Fields {
		kinds[field.Event] = append(kinds[field.Event], eventTypeProperty{
			Name: name,
			Type: field.ReturnType,
			Doc:  strings.TrimSpace(field.CommentText),
		})
	}

	eventTypes := make([]eventType, 0)
	for name, properties := range kinds {
		sort.Slice(properties, func(i, j int) bool {
			return properties[i].Name < properties[j].Name
		})

		info := extractVersionAndDefinition(module.EventTypeDocs[name])
		eventTypes = append(eventTypes, eventType{
			Name:             name,
			Definition:       info.Definition,
			Type:             info.Type,
			FromAgentVersion: info.FromAgentVersion,
			Properties:       properties,
		})
	}

	// for stability
	sort.Slice(eventTypes, func(i, j int) bool {
		return eventTypes[i].Name < eventTypes[j].Name
	})

	doc := documentation{
		Types: eventTypes,
	}

	res, err := prettyprint(doc)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(outputPath, res, 0644)
}

var (
	minVersionRE      = regexp.MustCompile(`^\[(?P<version>[0-9.]+)\]\s*\[(?P<type>\w+)\](?P<def>.*)`)
	minVersionREIndex = minVersionRE.SubexpIndex("version")
	typeREIndex       = minVersionRE.SubexpIndex("type")
	definitionREIndex = minVersionRE.SubexpIndex("def")
)

type eventTypeInfo struct {
	Definition       string
	Type             string
	FromAgentVersion string
}

func extractVersionAndDefinition(comment string) eventTypeInfo {
	trimmed := strings.TrimSpace(comment)

	if matches := minVersionRE.FindStringSubmatch(trimmed); matches != nil {
		return eventTypeInfo{
			Definition:       strings.TrimSpace(matches[definitionREIndex]),
			Type:             strings.TrimSpace(matches[typeREIndex]),
			FromAgentVersion: strings.TrimSpace(matches[minVersionREIndex]),
		}
	}

	return eventTypeInfo{
		Definition: trimmed,
	}
}

// JSONTypeMapper maps an internal serializers type to a user-facing documentation name
func JSONTypeMapper(ty reflect.Type) string {
	const selinuxPrefix = "selinux"

	base := strings.TrimSuffix(ty.Name(), "Serializer")
	if strings.HasPrefix(base, selinuxPrefix) {
		return "SELinux" + strings.TrimPrefix(base, selinuxPrefix)
	}

	return base
}
