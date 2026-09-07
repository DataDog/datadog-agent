// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/driver"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
)

type installerDescription struct {
	ID        string `json:"id"`
	Updatable bool   `json:"updatable"`
}

type environmentDescription struct {
	Base        string                 `json:"base"`
	Description string                 `json:"description"`
	Installers  []installerDescription `json:"installers"`
}

func cmdEnvironments(args []string) error {
	return listEnvironmentTypes(args, os.Stdout, os.Stderr)
}

// This lists registered types, not instances. It deliberately never opens the
// environment store or contacts an executor, Docker, Kubernetes or AWS.
func listEnvironmentTypes(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("environments", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "print registered environment types as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("environments takes no positional arguments")
	}

	environments := make([]environmentDescription, 0)
	for _, id := range driver.IDs() {
		d, err := driver.Get(id)
		if err != nil {
			return err
		}
		desc := environmentDescription{
			Base:        d.ID(),
			Description: d.Description(),
			Installers:  make([]installerDescription, 0),
		}
		for _, inst := range d.Installers() {
			_, updatable := inst.(installer.Updatable)
			desc.Installers = append(desc.Installers, installerDescription{ID: inst.ID(), Updatable: updatable})
		}
		slices.SortFunc(desc.Installers, func(a, b installerDescription) int { return strings.Compare(a.ID, b.ID) })
		environments = append(environments, desc)
	}
	if *asJSON {
		return json.NewEncoder(stdout).Encode(environments)
	}

	w := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "BASE\tINSTALLERS\tDESCRIPTION")
	for _, env := range environments {
		methods := make([]string, 0, len(env.Installers))
		for _, inst := range env.Installers {
			label := inst.ID
			if inst.Updatable {
				label += " (update)"
			}
			methods = append(methods, label)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", env.Base, strings.Join(methods, ", "), env.Description)
	}
	return w.Flush()
}

func cmdInit(args []string) error {
	return generateStarterConfig(args, os.Stdout, os.Stderr)
}

func generateStarterConfig(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	base := fs.String("base", "", "environment type (see e2ectl environments)")
	output := fs.String("output", "-", "new config file; '-' prints YAML to stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("init takes no positional arguments; use --base and --output")
	}
	if *base == "" {
		return fmt.Errorf("--base is required (see e2ectl environments)")
	}
	if *output == "" {
		return fmt.Errorf("--output must be a file path or '-' for stdout")
	}
	data, err := driver.StarterConfig(*base)
	if err != nil {
		return err
	}
	if *output == "-" {
		_, err := io.Copy(stdout, bytes.NewReader(data))
		return err
	}
	if err := writeNewConfig(*output, data); err != nil {
		return err
	}
	fmt.Fprintf(stderr, "Wrote %s. Review its settings before running e2ectl start.\n", *output)
	return nil
}

// Refuse to overwrite files or follow an existing symlink. The template has no
// credentials, but configs may later contain sensitive data, so create it private.
func writeNewConfig(path string, data []byte) (err error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("refusing to overwrite existing config %q; choose another --output path", path)
	}
	if err != nil {
		return fmt.Errorf("creating config %q: %w", path, err)
	}
	defer func() {
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	_, err = f.Write(data)
	return err
}
