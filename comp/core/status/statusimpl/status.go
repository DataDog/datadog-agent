// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package statusimpl implements the status component interface
package statusimpl

import (
	"bytes"
	"embed"
	"encoding/json"
	"io"
	"path"
	"sort"
	"text/template"
	"unicode"

	"go.uber.org/fx"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

//go:embed templates
var templatesFS embed.FS

type dependencies struct {
	fx.In
	Config config.Component
	Params status.Params

	Providers       []status.Provider       `group:"status"`
	HeaderProviders []status.HeaderProvider `group:"header_status"`
}

type statusImplementation struct {
	sortedHeaderProviders    []status.HeaderProvider
	sortedSectionNames       []string
	sortedProvidersBySection map[string][]status.Provider
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newStatus),
	)
}

func sortByName(providers []status.Provider) []status.Provider {
	sort.SliceStable(providers, func(i, j int) bool {
		return providers[i].Name() < providers[j].Name()
	})

	return providers
}

func newStatus(deps dependencies) (status.Component, error) {
	// Sections are sorted by name
	// The exception is the collector section. We want that to be the first section to be displayed
	// We manually insert the collector section in the first place after sorting them alphabetically
	sortedSectionNames := []string{}
	collectorSectionPresent := false

	for _, provider := range deps.Providers {
		if provider.Section() == status.CollectorSection && !collectorSectionPresent {
			collectorSectionPresent = true
		}

		if !present(provider.Section(), sortedSectionNames) && provider.Section() != status.CollectorSection {
			sortedSectionNames = append(sortedSectionNames, provider.Section())
		}
	}

	sort.Strings(sortedSectionNames)

	if collectorSectionPresent {
		sortedSectionNames = append([]string{status.CollectorSection}, sortedSectionNames...)
	}

	// Providers of each section are sort alphabetically by name
	sortedProvidersBySection := map[string][]status.Provider{}
	for _, provider := range deps.Providers {
		providers := sortedProvidersBySection[provider.Section()]
		sortedProvidersBySection[provider.Section()] = append(providers, provider)
	}
	for section, providers := range sortedProvidersBySection {
		sortedProvidersBySection[section] = sortByName(providers)
	}

	// Header providers are sorted by index
	// We manually insert the common header provider in the first place after sorting is done
	sortedHeaderProviders := []status.HeaderProvider{}
	sortedHeaderProviders = append(sortedHeaderProviders, deps.HeaderProviders...)

	sort.SliceStable(sortedHeaderProviders, func(i, j int) bool {
		return sortedHeaderProviders[i].Index() < sortedHeaderProviders[j].Index()
	})

	sortedHeaderProviders = append([]status.HeaderProvider{newCommonHeaderProvider(deps.Params, deps.Config)}, sortedHeaderProviders...)

	return &statusImplementation{
		sortedSectionNames:       sortedSectionNames,
		sortedProvidersBySection: sortedProvidersBySection,
		sortedHeaderProviders:    sortedHeaderProviders,
	}, nil
}

func (s *statusImplementation) GetStatus(format string, verbose bool, excludeSections ...string) ([]byte, error) {
	var errors []error

	switch format {
	case "json":
		stats := make(map[string]interface{})
		for _, sc := range s.sortedHeaderProviders {
			if present(sc.Name(), excludeSections) {
				continue
			}

			if err := sc.JSON(verbose, stats); err != nil {
				errors = append(errors, err)
			}
		}

		for _, providers := range s.sortedProvidersBySection {
			for _, provider := range providers {
				if present(provider.Section(), excludeSections) {
					continue
				}
				if err := provider.JSON(verbose, stats); err != nil {
					errors = append(errors, err)
				}
			}
		}

		if len(errors) > 0 {
			errorsInfo := []string{}
			for _, error := range errors {
				errorsInfo = append(errorsInfo, error.Error())
			}
			stats["errors"] = errorsInfo
		}

		return json.Marshal(stats)
	case "text":
		var b = new(bytes.Buffer)

		for _, sc := range s.sortedHeaderProviders {
			name := sc.Name()
			if present(name, excludeSections) {
				continue
			}

			if len(name) > 0 {
				printHeader(b, name)
				newLine(b)

				if err := sc.Text(verbose, b); err != nil {
					errors = append(errors, err)
				}

				newLine(b)
			}
		}

		for _, section := range s.sortedSectionNames {
			if present(section, excludeSections) {
				continue
			}

			if len(section) > 0 {
				printHeader(b, section)
				newLine(b)

				for _, provider := range s.sortedProvidersBySection[section] {
					if err := provider.Text(verbose, b); err != nil {
						errors = append(errors, err)
					}
				}

				newLine(b)
			}
		}
		if len(errors) > 0 {
			if err := renderErrors(b, errors); err != nil {
				return []byte{}, err
			}

			return b.Bytes(), nil
		}

		return b.Bytes(), nil
	case "html":
		var b = new(bytes.Buffer)

		for _, sc := range s.sortedHeaderProviders {
			if present(sc.Name(), excludeSections) {
				continue
			}

			err := sc.HTML(verbose, b)
			if err != nil {
				return b.Bytes(), err
			}
		}

		for _, section := range s.sortedSectionNames {
			if present(section, excludeSections) {
				continue
			}

			for _, provider := range s.sortedProvidersBySection[section] {
				err := provider.HTML(verbose, b)
				if err != nil {
					return b.Bytes(), err
				}
			}
		}
		return b.Bytes(), nil
	default:
		return []byte{}, nil
	}
}

func (s *statusImplementation) GetStatusBySection(section string, format string, verbose bool) ([]byte, error) {
	var errors []error

	switch section {
	case "header":
		providers := s.sortedHeaderProviders
		switch format {
		case "json":
			stats := make(map[string]interface{})

			for _, sc := range providers {
				if err := sc.JSON(verbose, stats); err != nil {
					errors = append(errors, err)
				}
			}

			if len(errors) > 0 {
				errorsInfo := []string{}
				for _, error := range errors {
					errorsInfo = append(errorsInfo, error.Error())
				}
				stats["errors"] = errorsInfo
			}

			return json.Marshal(stats)
		case "text":
			var b = new(bytes.Buffer)

			for i, sc := range providers {
				if i == 0 {
					printHeader(b, sc.Name())
					newLine(b)
				}

				err := sc.Text(verbose, b)
				if err != nil {
					errors = append(errors, err)
				}
			}

			newLine(b)

			if len(errors) > 0 {
				if err := renderErrors(b, errors); err != nil {
					return []byte{}, err
				}

				return b.Bytes(), nil
			}

			return b.Bytes(), nil
		case "html":
			var b = new(bytes.Buffer)

			for _, sc := range providers {
				err := sc.HTML(verbose, b)
				if err != nil {
					return b.Bytes(), err
				}
			}
			return b.Bytes(), nil
		default:
			return []byte{}, nil
		}
	default:
		providers := s.sortedProvidersBySection[section]
		switch format {
		case "json":
			stats := make(map[string]interface{})

			for _, sc := range providers {
				if err := sc.JSON(verbose, stats); err != nil {
					errors = append(errors, err)
				}
			}

			if len(errors) > 0 {
				errorsInfo := []string{}
				for _, error := range errors {
					errorsInfo = append(errorsInfo, error.Error())
				}
				stats["errors"] = errorsInfo
			}

			return json.Marshal(stats)
		case "text":
			var b = new(bytes.Buffer)

			for i, sc := range providers {
				if i == 0 {
					printHeader(b, section)
					newLine(b)
				}

				if err := sc.Text(verbose, b); err != nil {
					errors = append(errors, err)
				}
			}

			if len(errors) > 0 {
				if err := renderErrors(b, errors); err != nil {
					return []byte{}, err
				}

				return b.Bytes(), nil
			}

			return b.Bytes(), nil
		case "html":
			var b = new(bytes.Buffer)

			for _, sc := range providers {
				err := sc.HTML(verbose, b)
				if err != nil {
					return b.Bytes(), err
				}
			}
			return b.Bytes(), nil
		default:
			return []byte{}, nil
		}
	}
}

func present(value string, container []string) bool {
	for _, v := range container {
		if v == value {
			return true
		}
	}

	return false
}

func printHeader(buffer *bytes.Buffer, section string) {
	dashes := []byte(status.PrintDashes(section, "="))
	buffer.Write(dashes)
	newLine(buffer)

	runes := []rune(section)
	if unicode.IsUpper(runes[0]) {
		buffer.Write([]byte(section))
	} else {
		buffer.Write([]byte(cases.Title(language.Und).String(section)))
	}
	newLine(buffer)
	buffer.Write(dashes)
}

func newLine(buffer *bytes.Buffer) {
	buffer.Write([]byte("\n"))
}

func renderErrors(w io.Writer, errs []error) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("templates", "errors.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := template.Must(template.New("errors").Parse(string(tmpl)))
	return t.Execute(w, errs)
}
