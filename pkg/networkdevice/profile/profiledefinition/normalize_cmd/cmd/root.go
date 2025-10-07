// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cmd implements a cobra command for validating and normalizing profiles.
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "normalize_cmd FILE [FILE...]",
	Short: "Validate and normalize profiles.",
	Long: `normalize_cmd is a tool for validating and normalizing profiles.

	Each profile file passed in will be parsed, and any errors in them will be
	reported. If an output directory is specified with -o, then the profiles will
	also be normalized, migrating legacy and deprecated structures to their
	modern counterparts, and written to the output directory.`,

	Run: func(cmd *cobra.Command, args []string) {
		outdir, err := cmd.Flags().GetString("outdir")
		if err != nil {
			fmt.Printf("parse failure %v, unable to generate output\n", err)
		}
		useJSON, err := cmd.Flags().GetBool("json")
		if err != nil {
			fmt.Printf("parse failure %v, unable to generate output\n", err)
		}
		strict, err := cmd.Flags().GetBool("strict")
		if err != nil {
			fmt.Printf("parse failure %v, unable to generate output\n", err)
		}
		for _, filePath := range args {
			var name string
			if filePath == "-" {
				name = "stdin"
			} else {
				filename := filepath.Base(filePath)
				name = filename[:len(filename)-len(filepath.Ext(filename))] // remove extension
			}
			def, errors := GetProfile(filePath, strict)
			if len(errors) > 0 {
				fmt.Printf("*** %d error(s) in profile %q ***\n", len(errors), filePath)
				for _, e := range errors {
					fmt.Println("  ", e)
				}
				fmt.Println()
				continue
			}
			if err := WriteProfile(def, name, outdir, useJSON); err != nil {
				fmt.Println(err)
			}
		}
	},
}

// GetProfile parses a profile from a file path and validates it.
func GetProfile(filePath string, strict bool) (*profiledefinition.ProfileDefinition, []string) {
	var inFile io.Reader
	if filePath == "-" {
		inFile = os.Stdin
	} else {
		f, err := os.Open(filePath)
		if err != nil {
			return nil, []string{fmt.Sprintf("unable to read file: %v", err)}
		}
		defer func() {
			_ = f.Close()
		}()
		inFile = f
	}
	def := profiledefinition.NewProfileDefinition()
	dec := yaml.NewDecoder(inFile)
	dec.SetStrict(strict)
	err := dec.Decode(&def)
	if err != nil {
		return nil, []string{fmt.Sprintf("unable to parse profile: %v", err)}
	}
	errors := profiledefinition.ValidateEnrichProfile(def)
	if len(errors) > 0 {
		return nil, errors
	}
	return def, nil
}

// WriteProfile writes a profile to disk.
func WriteProfile(def *profiledefinition.ProfileDefinition, name string, outdir string, useJSON bool) error {
	if outdir == "" {
		return nil
	}
	var outPath string
	var data []byte
	if useJSON {
		var err error
		outPath = filepath.Join(outdir, name+".json")
		data, err = json.Marshal(def)
		if err != nil {
			return fmt.Errorf("unable to marshal profile %s: %w", name, err)
		}
	} else {
		var err error
		data, err = yaml.Marshal(def)
		outPath = filepath.Join(outdir, name+".yaml")
		if err != nil {
			return fmt.Errorf("unable to marshal profile %s: %w", name, err)
		}
	}
	var writer io.Writer
	if outdir == "-" {
		writer = os.Stdout
		outPath = "stdout"
	} else {
		f, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("unable to create file %s: %w", outPath, err)
		}
		defer func() {
			_ = f.Close()
		}()
		writer = f
	}
	_, err := writer.Write(data)
	if err != nil {
		return fmt.Errorf("unable to write to %s: %w", outPath, err)
	}
	return nil
}

// Execute runs the command.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringP("outdir", "o", "", "Output path for normalized files. If blank, inputs will be validated but not output.")
	rootCmd.Flags().BoolP("json", "j", false, "Output as JSON")
	rootCmd.Flags().BoolP("strict", "s", false, "Parse profile with strict parsing, so that e.g. unexpected fields will report an error instead of silently being ignored.")
}
