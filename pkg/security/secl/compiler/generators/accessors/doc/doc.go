// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package doc

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/generators/accessors/common"
)

const (
	generateConstantsAnnotationPrefix = "// generate_constants:"
)

type documentation struct {
	Types     []eventType `json:"event_types"`
	Constants []constants `json:"constants"`
}

type eventType struct {
	Name             string              `json:"name"`
	Definition       string              `json:"definition"`
	Type             string              `json:"type"`
	FromAgentVersion string              `json:"from_agent_version"`
	Experimental     bool                `json:"experimental"`
	Properties       []eventTypeProperty `json:"properties"`
}

type eventTypeProperty struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Doc       string `json:"definition"`
	Constants string `json:"constants"`
}

type constants struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	All         []constant `json:"all"`
}

type constant struct {
	Name         string `json:"name"`
	Architecture string `json:"architecture"`
}

func translateFieldType(rt string) string {
	switch rt {
	case "net.IPNet", "net.IP":
		return "IP/CIDR"
	}
	return rt
}

// GenerateDocJSON generates the SECL json documentation file to the provided outputPath
func GenerateDocJSON(module *common.Module, seclModelPath, outputPath string) error {
	// parse constants
	consts, err := parseConstants(seclModelPath, module.BuildTags)
	if err != nil {
		return fmt.Errorf("couldn't generate documentation for constants: %w", err)
	}

	kinds := make(map[string][]eventTypeProperty)

	for name, field := range module.Fields {
		// check if the constant exists
		if len(field.Constants) > 0 {
			var found bool
			for _, constantList := range consts {
				if constantList.Name == field.Constants {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("couldn't generate documentation for %s: unknown constant name %s", name, field.Constants)
			}
		}

		kinds[field.Event] = append(kinds[field.Event], eventTypeProperty{
			Name:      name,
			Type:      translateFieldType(field.ReturnType),
			Doc:       strings.TrimSpace(field.CommentText),
			Constants: field.Constants,
		})
	}

	eventTypes := make([]eventType, 0)
	for name, properties := range kinds {
		sort.Slice(properties, func(i, j int) bool {
			return properties[i].Name < properties[j].Name
		})

		info := extractVersionAndDefinition(module.EventTypes[name])
		eventTypes = append(eventTypes, eventType{
			Name:             name,
			Definition:       info.Definition,
			Type:             info.Type,
			FromAgentVersion: info.FromAgentVersion,
			Experimental:     info.Experimental,
			Properties:       properties,
		})
	}

	// for stability
	sort.Slice(eventTypes, func(i, j int) bool {
		return eventTypes[i].Name < eventTypes[j].Name
	})

	doc := documentation{
		Types:     eventTypes,
		Constants: consts,
	}

	res, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(outputPath, res, 0644)
}

func mergeConstants(one []constants, two []constants) []constants {
	var output []constants

	// add constants from one to output
	for _, consts := range one {
		output = append(output, consts)
	}

	// check if the constants from two should be appended or merged
	for _, constsTwo := range two {
		shouldAppendConsts := true
		for i, existingConsts := range output {
			if existingConsts.Name == constsTwo.Name {
				shouldAppendConsts = false

				// merge architecture if necessary
				for _, constTwo := range constsTwo.All {
					shouldAppendConst := true
					for j, existingConst := range existingConsts.All {
						if constTwo.Name == existingConst.Name {
							shouldAppendConst = false

							if len(constTwo.Architecture) > 0 && constTwo.Architecture != "all" && len(existingConst.Architecture) > 0 && existingConst.Architecture != "all" {
								existingConst.Architecture += ", " + constTwo.Architecture
							}
							output[i].All[j].Architecture = existingConst.Architecture
						}
					}
					if shouldAppendConst {
						output[i].All = append(output[i].All, constTwo)
					}
				}
				break
			}
		}
		if shouldAppendConsts {
			output = append(output, constsTwo)
		}
	}
	return output
}

func parseArchFromFilepath(filepath string) (string, error) {
	switch {
	case strings.HasSuffix(filepath, "common.go") || strings.HasSuffix(filepath, "linux.go"):
		return "all", nil
	case strings.HasSuffix(filepath, "amd64.go"):
		return "amd64", nil
	case strings.HasSuffix(filepath, "arm.go"):
		return "arm", nil
	case strings.HasSuffix(filepath, "arm64.go"):
		return "arm64", nil
	default:
		return "", fmt.Errorf("couldn't parse architecture from filepath: %s", filepath)
	}
}

func parseConstantsFile(filepath string, tags []string) ([]constants, error) {
	// extract architecture from filename
	arch, err := parseArchFromFilepath(filepath)
	if err != nil {
		return nil, err
	}

	// generate constants
	var output []constants
	cfg := packages.Config{
		Mode:       packages.NeedSyntax | packages.NeedTypes | packages.NeedImports,
		BuildFlags: []string{"-mod=mod", fmt.Sprintf("-tags=%s", tags)},
	}

	pkgs, err := packages.Load(&cfg, filepath)
	if err != nil {
		return nil, fmt.Errorf("load error:%w", err)
	}

	if len(pkgs) == 0 || len(pkgs[0].Syntax) == 0 {
		return nil, fmt.Errorf("couldn't parse constant file")
	}

	pkg := pkgs[0]
	astFile := pkg.Syntax[0]
	for _, decl := range astFile.Decls {
		if decl, ok := decl.(*ast.GenDecl); ok {
			for _, s := range decl.Specs {
				var consts constants
				val, ok := s.(*ast.ValueSpec)
				if !ok {
					continue
				}

				// check if this ValueSpec has a generate_commands annotation
				if val.Doc == nil {
					continue
				}
				for _, comment := range val.Doc.List {
					if !strings.HasPrefix(comment.Text, generateConstantsAnnotationPrefix) {
						continue
					}

					name, description, found := strings.Cut(strings.TrimPrefix(comment.Text, generateConstantsAnnotationPrefix), ",")
					if !found {
						continue
					}
					consts.Name = name
					consts.Description = description
					break
				}
				if len(consts.Name) == 0 || len(consts.Description) == 0 {
					continue
				}

				// extract list of keys
				if len(val.Values) < 1 {
					continue
				}
				values, ok := val.Values[0].(*ast.CompositeLit)
				if !ok {
					continue
				}
				for _, value := range values.Elts {
					valueExpr, ok := value.(*ast.KeyValueExpr)
					if !ok {
						continue
					}

					// create new constant entry from valueExpr
					consts.All = append(consts.All, constant{
						Name:         strings.Trim(valueExpr.Key.(*ast.BasicLit).Value, "\""),
						Architecture: arch,
					})
				}

				output = append(output, consts)
			}
		}
	}

	return output, nil
}

func parseConstants(path string, tags []string) ([]constants, error) {
	var output []constants
	for _, filename := range []string{"consts_common.go", "consts_linux.go", "consts_linux_amd64.go", "consts_linux_arm.go", "consts_linux_arm64.go"} {
		consts, err := parseConstantsFile(filepath.Join(path, filename), tags)
		if err != nil {
			return nil, err
		}

		output = mergeConstants(output, consts)
	}

	// sort the list of constants
	sort.Slice(output, func(i int, j int) bool {
		return output[i].Name < output[j].Name
	})
	return output, nil
}

var (
	minVersionRE        = regexp.MustCompile(`^\[(?P<version>(\w|\.|\s)*)\]\s*\[(?P<type>\w+)\]\s*(\[(?P<experimental>Experimental)\])?\s*(?P<def>.*)`)
	minVersionREIndex   = minVersionRE.SubexpIndex("version")
	typeREIndex         = minVersionRE.SubexpIndex("type")
	experimentalREIndex = minVersionRE.SubexpIndex("experimental")
	definitionREIndex   = minVersionRE.SubexpIndex("def")
)

type eventTypeInfo struct {
	Definition       string
	Type             string
	Experimental     bool
	FromAgentVersion string
}

func extractVersionAndDefinition(evtType *common.EventTypeMetadata) eventTypeInfo {
	var comment string
	if evtType != nil {
		comment = evtType.Doc
	}
	trimmed := strings.TrimSpace(comment)

	if matches := minVersionRE.FindStringSubmatch(trimmed); matches != nil {
		return eventTypeInfo{
			Definition:       strings.TrimSpace(matches[definitionREIndex]),
			Type:             strings.TrimSpace(matches[typeREIndex]),
			Experimental:     matches[experimentalREIndex] != "",
			FromAgentVersion: strings.TrimSpace(matches[minVersionREIndex]),
		}
	}

	return eventTypeInfo{
		Definition: trimmed,
	}
}
