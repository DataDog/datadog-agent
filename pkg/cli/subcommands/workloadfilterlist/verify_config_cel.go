// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cel

package workloadfilterlist

import (
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
	yaml "gopkg.in/yaml.v2"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/impl/parse"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/celprogram"
)

// verifyCELConfig validates CEL rules from a YAML file
func verifyCELConfig(_ io.Writer, reader io.Reader) error {
	fmt.Fprintf(color.Output, "\n%s Validating CEL Configuration\n", color.CyanString("->"))
	fmt.Fprintf(color.Output, "    %s\n", color.CyanString("Loading YAML file..."))

	data, err := io.ReadAll(reader)
	if err != nil {
		fmt.Fprintf(color.Output, "%s Failed to read input\n", color.HiRedString("✗"))
		return fmt.Errorf("failed to read input: %w", err)
	}

	var ruleBundles []workloadfilter.RuleBundle
	err = yaml.UnmarshalStrict(data, &ruleBundles)
	if err != nil {
		fmt.Fprintf(color.Output, "%s Failed to unmarshal YAML\n", color.HiRedString("✗"))
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	if len(ruleBundles) == 0 {
		fmt.Fprintf(color.Output, "%s No rules found in the input\n", color.HiRedString("✗"))
		return fmt.Errorf("no rules found in the input")
	}

	fmt.Fprintf(color.Output, "%s YAML loaded successfully (%d bundle(s))\n",
		color.HiGreenString("✓"), len(ruleBundles))

	fmt.Fprintf(color.Output, "\n%s Validating configuration structure...\n", color.CyanString("->"))
	productRules, parseErrors := parse.GetProductConfigs(ruleBundles)

	if parseErrors != nil {
		fmt.Fprintf(color.Output, "%s Configuration structure errors:\n", color.HiRedString("✗"))
		for _, err := range parseErrors {
			fmt.Fprintf(color.Output, "  - %s\n", color.RedString(err.Error()))
		}
		return fmt.Errorf("invalid configuration structure")
	}

	fmt.Fprintf(color.Output, "%s Configuration structure is valid\n", color.HiGreenString("✓"))

	// Compiling the CEL rules
	fmt.Fprintf(color.Output, "\n%s Compiling CEL rules...\n", color.CyanString("->"))
	hasErrors := false

	for product, resourceRules := range productRules {
		fmt.Fprintf(color.Output, "\n  %s %s\n", color.HiCyanString("->"), color.CyanString(string(product)))

		for resourceType, rules := range resourceRules {
			fmt.Fprintf(color.Output, "    %s %s (%d rule(s))\n", color.HiCyanString("Resource:"), string(resourceType), len(rules))

			rulesStr := strings.Join(rules, " || ")

			_, err := celprogram.CreateCELProgram(rulesStr, resourceType)
			if err != nil {
				hasErrors = true
				fmt.Fprintf(color.Output, "      %s Compilation failed: %s\n",
					color.HiRedString("✗"),
					color.RedString(err.Error()))

				for i, rule := range rules {
					fmt.Fprintf(color.Output, "        Rule %d: %s\n", i+1, rule)
				}
			} else {
				fmt.Fprintf(color.Output, "      %s All rules compiled successfully\n",
					color.HiGreenString("✓"))
			}
		}
	}

	fmt.Fprintln(color.Output)
	if hasErrors {
		fmt.Fprintf(color.Output, "%s Validation failed - some rules have errors\n",
			color.HiRedString("✗"))
		return fmt.Errorf("CEL compilation failed")
	}

	fmt.Fprintf(color.Output, "%s All rules are valid!\n", color.HiGreenString("✅"))
	return nil
}
