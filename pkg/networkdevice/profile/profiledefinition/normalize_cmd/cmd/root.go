// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "normalize_cmd",
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		outdir, err := cmd.Flags().GetString("outdir")
		if err != nil {
			fmt.Printf("parse failure %v, unable to generate output\n", err)
		}
		useJSON, err := cmd.Flags().GetBool("json")
		if err != nil {
			fmt.Printf("parse failure %v, unable to generate output\n", err)
		}
		for _, filePath := range args {
			filename := filepath.Base(filePath)
			name := filename[:len(filename)-len(filepath.Ext(filename))] // remove extension
			def, errors := GetProfile(filePath)
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

func GetProfile(filePath string) (*profiledefinition.ProfileDefinition, []string) {
	buf, err := os.ReadFile(filePath)
	if err != nil {
		return nil, []string{fmt.Sprintf("unable to read file: %v", err)}
	}
	def := profiledefinition.NewProfileDefinition()
	err = yaml.Unmarshal(buf, def)
	if err != nil {
		return nil, []string{fmt.Sprintf("unable to parse profile: %v", err)}
	}
	errors := profiledefinition.ValidateEnrichProfile(def)
	if len(errors) > 0 {
		return nil, errors
	}
	return def, nil
}

func WriteProfile(def *profiledefinition.ProfileDefinition, name string, outdir string, useJSON bool) error {
	if outdir == "" {
		return nil
	}
	var filename string
	var data []byte
	if useJSON {
		var err error
		filename = name + ".json"
		data, err = json.Marshal(def)
		if err != nil {
			return fmt.Errorf("unable to marshal profile %s: %w", name, err)
		}
	} else {
		var err error
		data, err = yaml.Marshal(def)
		filename = name + ".yaml"
		if err != nil {
			return fmt.Errorf("unable to marshal profile %s: %w", name, err)
		}
	}
	outfile := filepath.Join(outdir, filename)
	f, err := os.Create(outfile)
	if err != nil {
		return fmt.Errorf("unable to create file %s: %w", outfile, err)
	}
	defer f.Close()
	_, err = f.Write(data)
	if err != nil {
		return fmt.Errorf("unable to write to file %s: %w", outfile, err)
	}
	return nil
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.Flags().StringP("outdir", "o", "", "Output path for normalized files. If blank, inputs will be validated but not output.")
	rootCmd.Flags().BoolP("json", "j", false, "Output as JSON")
}
