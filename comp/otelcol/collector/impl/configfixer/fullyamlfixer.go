// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configfixer

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/comp/otelcol/collector/impl/configfixer/ollama"
)

type FullyAMLFixer struct {
	chat        *ollama.Chat
	config      string
	firstPrompt bool
}

func NewFullyAMLFixer(client *ollama.Client, systemPrompt string, currentConfig string) *FullyAMLFixer {
	chat := client.CreateChat(systemPrompt)
	return &FullyAMLFixer{
		chat:        chat,
		config:      currentConfig,
		firstPrompt: true,
	}
}

func (f *FullyAMLFixer) Fix(err error) (string, string, error) {
	var prompt string
	if f.firstPrompt {
		prompt = strings.ReplaceAll(fullyAMLInitialPrompt, "{{CONFIG}}", f.config)
		prompt = strings.ReplaceAll(prompt, "{{ERROR}}", err.Error())
	} else {
		prompt = strings.ReplaceAll(fullyAMLPrompt, "{{ERROR}}", err.Error())
	}
	f.firstPrompt = false

	response, err := f.chat.Generate(prompt, ollama.WithDebug(), ollama.WithTemperature(0))
	if err != nil {
		return "", "", fmt.Errorf("failed to generate response: %v", err)
	}

	fixedConfig := extractYAML(response.Message.Content, "yaml")
	fixedConfig = strings.TrimSpace(fixedConfig)
	//fixedConfig = strings.ReplaceAll(fixedConfig, "    ", "  ")

	return fixedConfig, "", nil
}

// extractYAML extracts YAML content from LLM response, returning only the last block
func extractYAML(response string, blockType string) string {
	// Try to find YAML blocks - get the last one
	yamlRegex := regexp.MustCompile("(?s)```" + blockType + "\\s*\\n(.*?)```")
	matches := yamlRegex.FindAllStringSubmatch(response, -1)
	if len(matches) > 0 {
		// Return the last match
		return strings.TrimSpace(matches[len(matches)-1][1])
	}

	// Try to find generic code blocks - get the last one
	codeRegex := regexp.MustCompile("(?s)```[^\\n]*\\n(.*?)```")
	matches = codeRegex.FindAllStringSubmatch(response, -1)
	if len(matches) > 0 {
		// Return the last match
		return strings.TrimSpace(matches[len(matches)-1][1])
	}

	// Return the whole response trimmed
	return strings.TrimSpace(response)
}

//go:embed prompts/fullyaml_prompt.txt
var fullyAMLPrompt string

//go:embed prompts/fullyaml_initial_prompt.txt
var fullyAMLInitialPrompt string
