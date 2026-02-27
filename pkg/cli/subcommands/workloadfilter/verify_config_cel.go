// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cel

package workloadfilterlist

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
	yaml "go.yaml.in/yaml/v2"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/impl/parse"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/celprogram"
)

// verifyCELConfig validates CEL rules from a YAML file
func verifyCELConfig(writer io.Writer, reader io.Reader) error {
	fmt.Fprintf(writer, "\n%s Validating CEL Configuration\n", color.CyanString("->"))
	fmt.Fprintf(writer, "    %s\n", color.CyanString("Loading configuration input..."))

	data, err := io.ReadAll(reader)
	if err != nil {
		fmt.Fprintf(writer, "%s Failed to read input\n", color.HiRedString("✗"))
		return fmt.Errorf("failed to read input: %w", err)
	}

	var ruleBundles []workloadfilter.RuleBundle

	// Try JSON first (more strict), then fall back to YAML
	err = json.Unmarshal(data, &ruleBundles)
	if err != nil {
		// If JSON fails, try YAML
		err = yaml.UnmarshalStrict(data, &ruleBundles)
		if err != nil {
			fmt.Fprintf(writer, "%s Failed to unmarshal input (tried JSON and YAML)\n", color.HiRedString("✗"))
			return fmt.Errorf("failed to parse input: %w", err)
		}
		fmt.Fprintf(writer, "%s YAML loaded successfully (%d bundle(s))\n",
			color.HiGreenString("✓"), len(ruleBundles))
	} else {
		fmt.Fprintf(writer, "%s JSON loaded successfully (%d bundle(s))\n",
			color.HiGreenString("✓"), len(ruleBundles))
	}

	if len(ruleBundles) == 0 {
		fmt.Fprintf(writer, "%s No rules found in the input\n", color.HiRedString("✗"))
		return errors.New("no rules found in the input")
	}

	fmt.Fprintf(writer, "\n%s Validating configuration structure...\n", color.CyanString("->"))
	productRules, parseErrors := parse.GetProductConfigs(ruleBundles)

	if parseErrors != nil {
		fmt.Fprintf(writer, "%s Configuration structure errors:\n", color.HiRedString("✗"))
		for _, err := range parseErrors {
			fmt.Fprintf(writer, "  - %s\n", color.RedString(err.Error()))
		}
		return errors.New("invalid configuration structure")
	}

	fmt.Fprintf(writer, "%s Configuration structure is valid\n", color.HiGreenString("✓"))

	// Compiling the CEL rules
	fmt.Fprintf(writer, "\n%s Compiling CEL rules...\n", color.CyanString("->"))
	hasErrors := false

	for product, resourceRules := range productRules {
		fmt.Fprintf(writer, "\n  %s %s\n", color.HiCyanString("->"), color.CyanString(string(product)))

		for resourceType, rules := range resourceRules {
			fmt.Fprintf(writer, "    %s %s (%d rule(s))\n", color.HiCyanString("Resource:"), string(resourceType), len(rules))

			rulesStr := strings.Join(rules, " || ")

			_, err := celprogram.CreateCELProgram(rulesStr, resourceType)
			if err != nil {
				hasErrors = true
				fmt.Fprintf(writer, "      %s Compilation failed: %s\n",
					color.HiRedString("✗"),
					color.RedString(err.Error()))

				for i, rule := range rules {
					fmt.Fprintf(writer, "        Rule %d: %s\n", i+1, rule)
				}
			} else {
				fmt.Fprintf(writer, "      %s All rules compiled successfully\n",
					color.HiGreenString("✓"))
			}
		}
	}

	fmt.Fprintln(writer)
	if hasErrors {
		fmt.Fprintf(writer, "%s Validation failed - some rules have errors\n",
			color.HiRedString("✗"))
		return errors.New("CEL compilation failed")
	}

	fmt.Fprintf(writer, "%s All rules are valid!\n", color.HiGreenString("✅"))
	return nil
}
