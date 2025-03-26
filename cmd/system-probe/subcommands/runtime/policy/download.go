// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package policy holds policy CLI subcommand related files
package policy

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

type downloadPolicyCliParams struct {
	*command.GlobalParams

	check      bool
	outputPath string
	source     string
}

// DownloadPolicyCommand returns the CLI command for "policy download"
func DownloadPolicyCommand(globalParams *command.GlobalParams) *cobra.Command {
	downloadPolicyArgs := &downloadPolicyCliParams{
		GlobalParams: globalParams,
	}

	downloadPolicyCmd := &cobra.Command{
		Use:   "download",
		Short: "Download policies",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(downloadPolicy,
				fx.Supply(downloadPolicyArgs),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath),
					SecretParams: secrets.NewDisabledParams(),
					LogParams:    log.ForOneShot("SYS-PROBE", "off", false)}),
				core.Bundle(),
			)
		},
	}

	downloadPolicyCmd.Flags().BoolVar(&downloadPolicyArgs.check, "check", false, "Check policies after downloading")
	downloadPolicyCmd.Flags().StringVar(&downloadPolicyArgs.outputPath, "output-path", "", "Output path for downloaded policies")
	downloadPolicyCmd.Flags().StringVar(&downloadPolicyArgs.source, "source", "all", `Specify wether should download the custom, default or all policies. allowed: "all", "default", "custom"`)

	return downloadPolicyCmd
}

func downloadPolicy(log log.Component, config config.Component, _ secrets.Component, downloadPolicyArgs *downloadPolicyCliParams) error {
	var outputFile *os.File

	apiKey := config.GetString("api_key")
	appKey := config.GetString("app_key")

	if apiKey == "" {
		return errors.New("API key is empty")
	}

	if appKey == "" {
		return errors.New("application key is empty")
	}

	site := config.GetString("site")
	if site == "" {
		site = "datadoghq.com"
	}

	var outputWriter io.Writer
	if downloadPolicyArgs.outputPath == "" || downloadPolicyArgs.outputPath == "-" {
		outputWriter = os.Stdout
	} else {
		f, err := os.Create(downloadPolicyArgs.outputPath)
		if err != nil {
			return err
		}
		defer f.Close()
		outputFile = f
		outputWriter = f
	}

	downloadURL := fmt.Sprintf("https://api.%s/api/v2/remote_config/products/cws/policy/download", site)
	fmt.Fprintf(os.Stderr, "Policy download url: %s\n", downloadURL)

	headers := map[string]string{
		"Content-Type":       "application/json",
		"DD-API-KEY":         apiKey,
		"DD-APPLICATION-KEY": appKey,
	}

	if av, err := version.Agent(); err == nil {
		headers["DD-AGENT-VERSION"] = av.GetNumberAndPre()
	}

	ctx := context.Background()

	client := http.Client{
		Transport: httputils.CreateHTTPTransport(config),
		Timeout:   10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}

	for header, value := range headers {
		req.Header.Add(header, value)
	}

	res, err := client.Do(req)
	if err != nil {
		return err
	}

	resBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if res.StatusCode != 200 {
		return fmt.Errorf("failed to download policies: %s (error code %d)", string(resBytes), res.StatusCode)
	}
	defer res.Body.Close()

	// Unzip the downloaded file containing both default and custom policies
	reader, err := zip.NewReader(bytes.NewReader(resBytes), int64(len(resBytes)))
	if err != nil {
		return err
	}

	var defaultPolicy []byte
	var customPolicies []string

	for _, file := range reader.File {
		if strings.HasSuffix(file.Name, ".policy") {
			pf, err := file.Open()
			if err != nil {
				return err
			}
			policyData, err := io.ReadAll(pf)
			pf.Close()
			if err != nil {
				return err
			}

			if file.Name == "default.policy" {
				defaultPolicy = policyData
			} else {
				customPolicies = append(customPolicies, string(policyData))
			}
		}
	}

	tempDir, err := os.MkdirTemp("", "policy_check")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	if err := os.WriteFile(path.Join(tempDir, "default.policy"), defaultPolicy, 0644); err != nil {
		return err
	}
	for i, customPolicy := range customPolicies {
		if err := os.WriteFile(path.Join(tempDir, fmt.Sprintf("custom%d.policy", i+1)), []byte(customPolicy), 0644); err != nil {
			return err
		}
	}

	if downloadPolicyArgs.check {
		if err := CheckPolicies(log, config, &checkPoliciesCliParams{dir: tempDir}); err != nil {
			return err
		}
	}

	// Extract and merge rules from custom policies
	var customRules string
	for _, customPolicy := range customPolicies {
		customPolicyLines := strings.Split(customPolicy, "\n")
		rulesIndex := -1
		for i, line := range customPolicyLines {
			if strings.TrimSpace(line) == "rules:" {
				rulesIndex = i
				break
			}
		}
		if rulesIndex != -1 && rulesIndex+1 < len(customPolicyLines) {
			customRules += "\n" + strings.Join(customPolicyLines[rulesIndex+1:], "\n")
		}
	}

	// Output depending on user's specification
	var outputContent string
	switch downloadPolicyArgs.source {
	case "all":
		outputContent = string(defaultPolicy) + customRules
	case "default":
		outputContent = string(defaultPolicy)
	case "custom":
		outputContent = string(customRules)
	default:
		return errors.New("invalid source specified")
	}

	_, err = outputWriter.Write([]byte(outputContent))
	if err != nil {
		return err
	}

	if outputFile != nil {
		return outputFile.Close()
	}

	return err
}
