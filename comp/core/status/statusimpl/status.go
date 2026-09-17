// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package statusimpl implements the status component interface
package statusimpl

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"
	"unicode"

	"go.uber.org/fx"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	template "github.com/DataDog/datadog-agent/pkg/template/text"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

//go:embed templates
var templatesFS embed.FS

type dependencies struct {
	fx.In
	Config config.Component
	Params status.Params
	Log    log.Component

	Providers       []status.Provider       `group:"status"`
	HeaderProviders []status.HeaderProvider `group:"header_status"`
}

type provides struct {
	fx.Out

	Comp              status.Component
	FlareProvider     flaretypes.Provider
	APIGetStatus      api.AgentEndpointProvider
	APIGetSection     api.AgentEndpointProvider
	APIGetSectionList api.AgentEndpointProvider
}

type statusImplementation struct {
	sortedHeaderProviders    []status.HeaderProvider
	sortedSectionNames       []string
	sortedProvidersBySection map[string][]status.Provider
	dynamicSectionProviders  []status.DynamicSectionProvider
	log                      log.Component
}

type dynamicSectionAdapter struct {
	provider status.DynamicSectionProvider
	section  string
}

type jsonProvider interface {
	Name() string
	JSON(bool, map[string]interface{}) error
}

func (a dynamicSectionAdapter) Name() string {
	return a.provider.Name()
}

func (a dynamicSectionAdapter) Section() string {
	return a.section
}

func (a dynamicSectionAdapter) JSON(verbose bool, stats map[string]interface{}) error {
	return a.provider.JSONBySection(a.section, verbose, stats)
}

func (a dynamicSectionAdapter) Text(verbose bool, buffer io.Writer) error {
	return a.provider.TextBySection(a.section, verbose, buffer)
}

func (a dynamicSectionAdapter) HTML(verbose bool, buffer io.Writer) error {
	return a.provider.HTMLBySection(a.section, verbose, buffer)
}

// populateJSON preserves the shared map used by local providers, then merges
// dynamic providers without allowing them to overwrite local status fields.
func populateJSON(stats map[string]interface{}, providers []jsonProvider, verbose bool) []error {
	var errs []error
	var dynamicProviders []jsonProvider
	for _, provider := range providers {
		switch provider.(type) {
		case status.DynamicSectionProvider, dynamicSectionAdapter:
			dynamicProviders = append(dynamicProviders, provider)
		default:
			if err := provider.JSON(verbose, stats); err != nil {
				errs = append(errs, err)
			}
		}
	}
	for _, provider := range dynamicProviders {
		errs = append(errs, mergeDynamicProviderJSON(stats, provider, verbose)...)
	}
	return errs
}

func mergeDynamicProviderJSON(stats map[string]interface{}, provider jsonProvider, verbose bool) []error {
	providerStats := make(map[string]interface{})
	var errs []error
	if err := provider.JSON(verbose, providerStats); err != nil {
		errs = append(errs, err)
	}

	keys := make([]string, 0, len(providerStats))
	for key := range providerStats {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if _, exists := stats[key]; exists {
			errs = append(errs, fmt.Errorf("duplicate status JSON key %q from provider %q", key, provider.Name()))
			continue
		}
		stats[key] = providerStats[key]
	}

	return errs
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

func newStatus(deps dependencies) provides {
	// Sections are sorted by name
	// The exception is the collector section. We want that to be the first section to be displayed
	// We manually insert the collector section in the first place after sorting them alphabetically
	sortedSectionNames := []string{}
	collectorSectionPresent := false

	providers := fxutil.GetAndFilterGroup(deps.Providers)
	dynamicSectionProviders := make([]status.DynamicSectionProvider, 0)

	for _, provider := range providers {
		if dynamicProvider, ok := provider.(status.DynamicSectionProvider); ok {
			dynamicSectionProviders = append(dynamicSectionProviders, dynamicProvider)
		}

		if provider.Section() == status.CollectorSection && !collectorSectionPresent {
			collectorSectionPresent = true
		}

		if !present(provider.Section(), sortedSectionNames) && provider.Section() != status.CollectorSection {
			sortedSectionNames = append(sortedSectionNames, strings.ToLower(provider.Section()))
		}
	}

	sort.Strings(sortedSectionNames)

	if collectorSectionPresent {
		sortedSectionNames = append([]string{status.CollectorSection}, sortedSectionNames...)
	}

	// Providers of each section are sort alphabetically by name
	// Section names are stored lower case
	sortedProvidersBySection := map[string][]status.Provider{}
	for _, provider := range providers {
		lowerSectionName := strings.ToLower(provider.Section())
		providers := sortedProvidersBySection[lowerSectionName]
		sortedProvidersBySection[lowerSectionName] = append(providers, provider)
	}
	for section, providers := range sortedProvidersBySection {
		sortedProvidersBySection[section] = sortByName(providers)
	}

	// Header providers are sorted by index
	// We manually insert the common header provider in the first place after sorting is done
	sortedHeaderProviders := []status.HeaderProvider{}
	sortedHeaderProviders = append(sortedHeaderProviders, fxutil.GetAndFilterGroup(deps.HeaderProviders)...)

	sort.SliceStable(sortedHeaderProviders, func(i, j int) bool {
		return sortedHeaderProviders[i].Index() < sortedHeaderProviders[j].Index()
	})

	sortedHeaderProviders = append([]status.HeaderProvider{newCommonHeaderProvider(deps.Params, deps.Config)}, sortedHeaderProviders...)

	c := &statusImplementation{
		sortedSectionNames:       sortedSectionNames,
		sortedProvidersBySection: sortedProvidersBySection,
		sortedHeaderProviders:    sortedHeaderProviders,
		dynamicSectionProviders:  dynamicSectionProviders,
		log:                      deps.Log,
	}

	return provides{
		Comp:          c,
		FlareProvider: flaretypes.NewProvider(c.fillFlare),
		APIGetStatus: api.NewAgentEndpointProvider(
			func(w http.ResponseWriter, r *http.Request) { c.getStatus(w, r, "") },
			"/status",
			"GET",
		),
		APIGetSection: api.NewAgentEndpointProvider(
			c.getSection,
			"/status/section/{component}",
			"GET",
		),
		APIGetSectionList: api.NewAgentEndpointProvider(
			c.getSections,
			"/status/sections",
			"GET",
		),
	}
}

func (s *statusImplementation) GetStatus(format string, verbose bool, excludeSections ...string) ([]byte, error) {
	var errs []error

	switch format {
	case "json":
		stats := make(map[string]interface{})
		var providers []jsonProvider
		for _, sc := range s.sortedHeaderProviders {
			if present(sc.Name(), excludeSections) {
				continue
			}

			providers = append(providers, sc)
		}

		for _, section := range s.sortedSectionNames {
			for _, provider := range s.sortedProvidersBySection[section] {
				if present(provider.Section(), excludeSections) {
					continue
				}
				providers = append(providers, provider)
			}
		}
		errs = append(errs, populateJSON(stats, providers, verbose)...)

		if len(errs) > 0 {
			errorsInfo := []string{}
			for _, error := range errs {
				errorsInfo = append(errorsInfo, error.Error())
			}
			stats["errors"] = errorsInfo
		}

		return json.Marshal(stats)
	case "text":
		b := new(bytes.Buffer)

		for _, sc := range s.sortedHeaderProviders {
			name := sc.Name()
			if present(name, excludeSections) {
				continue
			}

			if len(name) > 0 {
				printHeader(b, name)
				newLine(b)

				if err := sc.Text(verbose, b); err != nil {
					errs = append(errs, err)
				}

				newLine(b)
			}
		}

		for _, section := range s.sortedSectionNames {
			if present(section, excludeSections) {
				continue
			}

			if len(section) > 0 {
				headerBuffer := new(bytes.Buffer)
				sectionBuffer := new(bytes.Buffer)

				for i, provider := range s.sortedProvidersBySection[section] {

					if i == 0 {
						printHeader(headerBuffer, provider.Section())
						newLine(headerBuffer)
					}
					if err := provider.Text(verbose, sectionBuffer); err != nil {
						errs = append(errs, err)
					}
				}

				if sectionBuffer.Len() == 0 {
					continue
				}

				b.Write(headerBuffer.Bytes())
				b.Write(sectionBuffer.Bytes())
				newLine(b)
			}
		}
		if len(errs) > 0 {
			if err := renderErrors(b, errs); err != nil {
				return []byte{}, err
			}

			return b.Bytes(), nil
		}

		return b.Bytes(), nil
	case "html":
		b := new(bytes.Buffer)

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

func (s *statusImplementation) GetStatusBySections(sections []string, format string, verbose bool) ([]byte, error) {
	var errs []error

	if len(sections) == 1 && sections[0] == "header" {
		providers := s.sortedHeaderProviders
		switch format {
		case "json":
			stats := make(map[string]interface{})

			for _, sc := range providers {
				if err := sc.JSON(verbose, stats); err != nil {
					errs = append(errs, err)
				}
			}

			if len(errs) > 0 {
				errorsInfo := []string{}
				for _, error := range errs {
					errorsInfo = append(errorsInfo, error.Error())
				}
				stats["errors"] = errorsInfo
			}

			return json.Marshal(stats)
		case "text":
			b := new(bytes.Buffer)

			for i, sc := range providers {
				if i == 0 {
					printHeader(b, sc.Name())
					newLine(b)
				}

				err := sc.Text(verbose, b)
				if err != nil {
					errs = append(errs, err)
				}
			}

			newLine(b)

			if len(errs) > 0 {
				if err := renderErrors(b, errs); err != nil {
					return []byte{}, err
				}

				return b.Bytes(), nil
			}

			return b.Bytes(), nil
		case "html":
			b := new(bytes.Buffer)

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

	// Get provider lists from one or more sections
	var providers []status.Provider
	for _, section := range sections {
		providersForSection, ok := s.providersForSection(section)
		if !ok {
			res, _ := json.Marshal(s.GetSections())
			errorMsg := fmt.Sprintf("unknown status section '%s', available sections are: %s", section, string(res))
			return nil, errors.New(errorMsg)
		}
		providers = append(providers, providersForSection...)
	}

	switch format {
	case "json":
		stats := make(map[string]interface{})
		jsonProviders := make([]jsonProvider, 0, len(providers))
		for _, sc := range providers {
			jsonProviders = append(jsonProviders, sc)
		}
		errs = append(errs, populateJSON(stats, jsonProviders, verbose)...)

		if len(errs) > 0 {
			errorsInfo := []string{}
			for _, error := range errs {
				errorsInfo = append(errorsInfo, error.Error())
			}
			stats["errors"] = errorsInfo
		}

		return json.Marshal(stats)
	case "text":
		b := new(bytes.Buffer)

		for i, sc := range providers {
			if i == 0 {
				printHeader(b, sc.Section())
				newLine(b)
			}

			if err := sc.Text(verbose, b); err != nil {
				errs = append(errs, err)
			}
		}

		if len(errs) > 0 {
			if err := renderErrors(b, errs); err != nil {
				return []byte{}, err
			}

			return b.Bytes(), nil
		}

		return b.Bytes(), nil
	case "html":
		b := new(bytes.Buffer)

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

func (s *statusImplementation) providersForSection(section string) ([]status.Provider, bool) {
	if providers, ok := s.sortedProvidersBySection[strings.ToLower(section)]; ok {
		return providers, true
	}

	providers := make([]status.Provider, 0)
	for _, provider := range s.dynamicSectionProviders {
		for _, dynamicSection := range provider.Sections() {
			if strings.EqualFold(section, dynamicSection) {
				providers = append(providers, dynamicSectionAdapter{
					provider: provider,
					section:  dynamicSection,
				})
				break
			}
		}
	}

	if len(providers) == 0 {
		return nil, false
	}

	return sortByName(providers), true
}

func (s *statusImplementation) GetSections() []string {
	sectionNames := make([]string, 0, len(s.sortedSectionNames))
	for _, section := range s.sortedSectionNames {
		if section != "header" && !present(section, sectionNames) {
			sectionNames = append(sectionNames, strings.ToLower(section))
		}
	}

	for _, provider := range s.dynamicSectionProviders {
		for _, section := range provider.Sections() {
			section = strings.ToLower(section)
			if section != "header" && !present(section, sectionNames) {
				sectionNames = append(sectionNames, section)
			}
		}
	}

	sort.Strings(sectionNames)
	sections := []string{"header"}
	if present(status.CollectorSection, sectionNames) {
		sections = append(sections, status.CollectorSection)
	}
	for _, section := range sectionNames {
		if section != status.CollectorSection {
			sections = append(sections, section)
		}
	}

	return sections
}

// fillFlare add the status.log to flares.
func (s *statusImplementation) fillFlare(_ context.Context, fb flaretypes.FlareBuilder) error {
	fb.AddFileFromFunc("status.log", func() ([]byte, error) { return s.GetStatus("text", true) }) //nolint:errcheck
	return nil
}

func present(value string, container []string) bool {
	valueLower := strings.ToLower(value)

	for _, v := range container {
		if strings.ToLower(v) == valueLower {
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
