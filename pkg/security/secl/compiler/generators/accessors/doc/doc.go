// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package doc holds doc related files
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
	SECLDocForLength                  = "SECLDoc[length] Definition:`Length of the corresponding element`" // SECLDocForLength defines SECL doc for length

)

type documentation struct {
	Types         []eventType             `json:"event_types"`
	PropertiesDoc []propertyDocumentation `json:"properties_doc"`
	Constants     []constants             `json:"constants"`
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
	Name        string `json:"name"`
	Definition  string `json:"definition"`
	DocLink     string `json:"property_doc_link"`
	PropertyKey string `json:"-"`
}

type constants struct {
	Name        string     `json:"name"`
	Link        string     `json:"link"`
	Description string     `json:"description"`
	All         []constant `json:"all"`
}

type constant struct {
	Name         string `json:"name"`
	Architecture string `json:"architecture"`
}

type example struct {
	Expression  string `json:"expression"`
	Description string `json:"description"`
}

type propertyDocumentation struct {
	Name                  string    `json:"name"`
	Link                  string    `json:"link"`
	Type                  string    `json:"type"`
	Doc                   string    `json:"definition"`
	Prefixes              []string  `json:"prefixes"`
	Constants             string    `json:"constants"`
	ConstantsLink         string    `json:"constants_link"`
	Examples              []example `json:"examples"`
	IsUniqueEventProperty bool      `json:"-"`
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
	cachedDocumentation := make(map[string]*propertyDocumentation)

	for name, field := range module.Fields {
		if field.GettersOnly {
			continue
		}

		// we currently don't want to publicly document the on-demand event type
		if field.Event == "ondemand" {
			continue
		}

		var propertyKey string
		var propertySuffix string
		var propertyDefinition string
		if strings.HasPrefix(field.Alias, field.AliasPrefix) {
			propertySuffix = strings.TrimPrefix(field.Alias, field.AliasPrefix)
			propertyKey = field.Struct + propertySuffix
			propertySuffix = strings.TrimPrefix(propertySuffix, ".")
		} else {
			propertyKey = field.Alias
			propertySuffix = field.Alias
		}

		if propertyDoc, exists := cachedDocumentation[propertyKey]; !exists {
			definition, constantsName, examples := parseSECLDocWithSuffix(field.CommentText, propertySuffix)
			if definition == "" {
				return fmt.Errorf("failed to parse SECL documentation for field '%s' (name:%s psuffix:%s, pkey:%s, alias:%s, aliasprefix:%s)\n%+v", name, field.Name, propertySuffix, propertyKey, field.Alias, field.AliasPrefix, field)
			}

			var constsLink string
			if len(constantsName) > 0 {
				var found bool
				for _, constantList := range consts {
					if constantList.Name == constantsName {
						found = true
						constsLink = constantList.Link
					}
				}
				if !found {
					return fmt.Errorf("couldn't generate documentation for %s: unknown constant name %s", name, constantsName)
				}
			}

			var propertyDoc = &propertyDocumentation{
				Name:                  name,
				Link:                  strings.ReplaceAll(name, ".", "-") + "-doc",
				Type:                  translateFieldType(field.ReturnType),
				Doc:                   strings.TrimSpace(definition),
				Prefixes:              []string{field.AliasPrefix},
				Constants:             constantsName,
				ConstantsLink:         constsLink,
				Examples:              make([]example, 0), // force the serialization of an empty array
				IsUniqueEventProperty: true,
			}
			propertyDoc.Examples = append(propertyDoc.Examples, examples...)
			propertyDefinition = propertyDoc.Doc
			cachedDocumentation[propertyKey] = propertyDoc
		} else if propertyDoc.IsUniqueEventProperty {
			propertyDoc.IsUniqueEventProperty = false
			fieldSuffix := strings.TrimPrefix(field.Alias, field.AliasPrefix)
			propertyDoc.Name = "*" + fieldSuffix
			propertyDoc.Link = "common-" + strings.ReplaceAll(strings.ToLower(propertyKey), ".", "-") + "-doc"
			propertyDoc.Prefixes = append(propertyDoc.Prefixes, field.AliasPrefix)
			propertyDefinition = propertyDoc.Doc
		} else {
			propertyDoc.Prefixes = append(propertyDoc.Prefixes, field.AliasPrefix)
			propertyDefinition = propertyDoc.Doc
		}

		if len(field.RestrictedTo) > 0 {
			for _, evt := range field.RestrictedTo {
				kinds[evt] = append(kinds[evt], eventTypeProperty{
					Name:        name,
					Definition:  propertyDefinition,
					PropertyKey: propertyKey,
				})
			}
		} else {
			eventType := field.Event
			if eventType == "" {
				eventType = "*"
			}

			kinds[eventType] = append(kinds[eventType], eventTypeProperty{
				Name:        name,
				Definition:  propertyDefinition,
				PropertyKey: propertyKey,
			})
		}
	}

	eventTypes := make([]eventType, 0)
	for name, properties := range kinds {
		for i := 0; i < len(properties); i++ {
			property := &properties[i]
			if propertyDoc, exists := cachedDocumentation[property.PropertyKey]; exists {
				property.DocLink = propertyDoc.Link
				sort.Slice(propertyDoc.Prefixes, func(i, j int) bool {
					return propertyDoc.Prefixes[i] < propertyDoc.Prefixes[j]
				})
			}
		}

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

	// force the serialization of an empty array
	propertiesDoc := make([]propertyDocumentation, 0)
	for _, ex := range cachedDocumentation {
		propertiesDoc = append(propertiesDoc, *ex)
	}
	sort.Slice(propertiesDoc, func(i, j int) bool {
		if propertiesDoc[i].Name != propertiesDoc[j].Name {
			return propertiesDoc[i].Name < propertiesDoc[j].Name
		}
		return propertiesDoc[i].Link < propertiesDoc[j].Link
	})

	doc := documentation{
		Types:         eventTypes,
		PropertiesDoc: propertiesDoc,
		Constants:     consts,
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
	output = append(output, one...)

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
	case strings.HasSuffix(filepath, "common.go") || strings.HasSuffix(filepath, "linux.go") || strings.HasSuffix(filepath, "windows.go"):
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

var nonLinkCharactersRegex = regexp.MustCompile(`(?:^[^a-z0-9]+)|(?:[^a-z0-9-]+)|(?:[^a-z0-9]+$)`)

func constsLinkFromName(constName string) string {
	return nonLinkCharactersRegex.ReplaceAllString(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(constName)), " ", "-"), "")
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
					consts.Link = constsLinkFromName(name)
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

	files := []string{"consts_common.go", "consts_linux.go", "consts_linux_amd64.go", "consts_linux_arm.go", "consts_linux_arm64.go"}
	for _, tag := range tags {
		if strings.Contains(tag, "windows") {
			files = []string{"consts_common.go", "consts_windows.go"}
			break
		}
	}

	for _, filename := range files {
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

var (
	seclDocRE  = regexp.MustCompile(`SECLDoc\[((?:[a-z0-9_]+\.?)*[a-z0-9_]+)\]\s*Definition:\s*\x60([^\x60]+)\x60\s*(?:Constants:\x60([^\x60]+)\x60\s*)?(?:Example:\s*\x60([^\x60]+)\x60\s*(?:Description:\s*\x60([^\x60]+)\x60\s*)?)*`)
	examplesRE = regexp.MustCompile(`Example:\s*\x60([^\x60]+)\x60\s*(?:Description:\s*\x60([^\x60]+)\x60\s*)?`)
)

func parseSECLDocWithSuffix(comment string, wantedSuffix string) (string, string, []example) {
	trimmed := strings.TrimSpace(comment)

	for _, match := range seclDocRE.FindAllStringSubmatchIndex(trimmed, -1) {
		matchedSubString := trimmed[match[0]:match[1]]
		seclSuffix := trimmed[match[2]:match[3]]
		if seclSuffix != wantedSuffix {
			continue
		}

		definition := trimmed[match[4]:match[5]]
		var constants string
		if match[6] != -1 && match[7] != -1 {
			constants = trimmed[match[6]:match[7]]
		}

		var examples []example
		for _, exampleMatch := range examplesRE.FindAllStringSubmatchIndex(matchedSubString, -1) {
			expr := matchedSubString[exampleMatch[2]:exampleMatch[3]]
			var desc string
			if exampleMatch[4] != -1 && exampleMatch[5] != -1 {
				desc = matchedSubString[exampleMatch[4]:exampleMatch[5]]
			}
			examples = append(examples, example{Expression: expr, Description: desc})
		}

		return strings.TrimSpace(definition), strings.TrimSpace(constants), examples
	}

	return "", "", nil
}
