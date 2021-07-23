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
	"regexp"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/generators/accessors/common"
)

// easyjson:json
type Documentation struct {
	Kinds []DocEventKind `json:"secl"`
}

// easyjson:json
type DocEventKind struct {
	Name             string             `json:"name"`
	Definition       string             `json:"definition"`
	Type             string             `json:"type"`
	FromAgentVersion string             `json:"from_agent_version"`
	Properties       []DocEventProperty `json:"properties"`
}

// easyjson:json
type DocEventProperty struct {
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

func GenerateDocJSON(module *common.Module, outputPath string) error {
	kinds := make(map[string][]DocEventProperty)

	for name, field := range module.Fields {
		kinds[field.Event] = append(kinds[field.Event], DocEventProperty{
			Name: name,
			Type: field.ReturnType,
			Doc:  strings.TrimSpace(field.CommentText),
		})
	}

	docKinds := make([]DocEventKind, 0)
	for name, properties := range kinds {
		sort.Slice(properties, func(i, j int) bool {
			return properties[i].Name < properties[j].Name
		})

		info := extractVersionAndDefinition(module.EventTypeDocs[name])
		docKinds = append(docKinds, DocEventKind{
			Name:             name,
			Definition:       info.Definition,
			Type:             info.Type,
			FromAgentVersion: info.FromAgentVersion,
			Properties:       properties,
		})
	}

	// for stability
	sort.Slice(docKinds, func(i, j int) bool {
		return docKinds[i].Name < docKinds[j].Name
	})

	doc := Documentation{
		Kinds: docKinds,
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

type EventTypeInfo struct {
	Definition       string
	Type             string
	FromAgentVersion string
}

func extractVersionAndDefinition(comment string) EventTypeInfo {
	trimmed := strings.TrimSpace(comment)

	if matches := minVersionRE.FindStringSubmatch(trimmed); matches != nil {
		return EventTypeInfo{
			Definition:       strings.TrimSpace(matches[definitionREIndex]),
			Type:             strings.TrimSpace(matches[typeREIndex]),
			FromAgentVersion: strings.TrimSpace(matches[minVersionREIndex]),
		}
	}

	return EventTypeInfo{
		Definition: trimmed,
	}
}
