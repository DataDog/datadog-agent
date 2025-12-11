// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configfixer

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/DataDog/datadog-agent/comp/otelcol/collector/impl/configfixer/ollama"
)

const SystemPrompt = "You are an OpenTelemetry Collector configuration expert"

func FixConfig(configPath string, configValidator func() error) error {
	configErr := configValidator()
	if configErr == nil {
		return nil
	}

	configContent, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	err = copyFile(configPath, configPath+".back")
	if err != nil {
		return fmt.Errorf("failed to copy config file: %w", err)
	}

	endpoint := "http://localhost:11434"
	modelName := "Qwen2.5-0.5B-Instruct-final-q8_0:latest"
	client := ollama.NewClient(endpoint, modelName)

	fixer := NewFullyAMLFixer(client, SystemPrompt, string(configContent))

	for i := 0; i < 3; i++ {
		fmt.Println("Fixing config file: ", configErr.Error())
		fixedConfig, _, err := fixer.Fix(configErr)
		if err != nil {
			return fmt.Errorf("failed to fix config file: %w", err)
		}

		err = os.WriteFile(configPath, []byte(fixedConfig), 0644)
		if err != nil {
			return fmt.Errorf("failed to write config file: %w", err)
		}

		configErr = configValidator()
		if configErr == nil {
			return nil
		}
	}
	return configErr
}

func copyFile(src, dst string) error {
	input, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(dst, input, 0644)
	if err != nil {
		return err
	}
	return nil
}
