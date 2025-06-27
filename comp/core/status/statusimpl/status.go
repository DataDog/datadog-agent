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
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
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

	Providers              []status.Provider                `group:"status"`
	HeaderProviders        []status.HeaderProvider          `group:"header_status"`
	DynamicProviders       []func() []status.Provider       `group:"dyn_status"`
	DynamicHeaderProviders []func() []status.HeaderProvider `group:"dyn_header_status"`
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
	log       log.Component
	providers statusProviderManager
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newStatus),
	)
}

func newStatus(deps dependencies) provides {
	// Create a provider getter that will handle the static and dynamic providers
	providers := newProviderGetter(
		deps.Log,
		newCommonHeaderProvider(deps.Params, deps.Config),
		fxutil.GetAndFilterGroup(deps.HeaderProviders),
		fxutil.GetAndFilterGroup(deps.Providers),
		fxutil.GetAndFilterGroup(deps.DynamicHeaderProviders),
		fxutil.GetAndFilterGroup(deps.DynamicProviders),
	)

	c := &statusImplementation{
		providers: providers,
		log:       deps.Log,
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
			"/{component}/status",
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
		for _, sc := range s.providers.SortedHeaderProviders() {
			if present(sc.Name(), excludeSections) {
				continue
			}

			if err := sc.JSON(verbose, stats); err != nil {
				errs = append(errs, err)
			}
		}

		for _, providers := range s.providers.SortedProvidersBySection() {
			for _, provider := range providers {
				if present(provider.Section(), excludeSections) {
					continue
				}
				if err := provider.JSON(verbose, stats); err != nil {
					errs = append(errs, err)
				}
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

		for _, sc := range s.providers.SortedHeaderProviders() {
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

		for _, section := range s.providers.SortedSectionNames() {
			if present(section, excludeSections) {
				continue
			}

			if len(section) > 0 {
				headerBuffer := new(bytes.Buffer)
				sectionBuffer := new(bytes.Buffer)

				for i, provider := range s.providers.SortedProvidersBySection()[section] {

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

		for _, sc := range s.providers.SortedHeaderProviders() {
			if present(sc.Name(), excludeSections) {
				continue
			}

			err := sc.HTML(verbose, b)
			if err != nil {
				return b.Bytes(), err
			}
		}

		for _, section := range s.providers.SortedSectionNames() {
			if present(section, excludeSections) {
				continue
			}

			for _, provider := range s.providers.SortedProvidersBySection()[section] {
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
		providers := s.providers.SortedHeaderProviders()
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
		providersForSection, ok := s.providers.SortedProvidersBySection()[strings.ToLower(section)]
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

func (s *statusImplementation) GetSections() []string {
	return append([]string{"header"}, s.providers.SortedSectionNames()...)
}

// fillFlare add the status.log to flares.
func (s *statusImplementation) fillFlare(fb flaretypes.FlareBuilder) error {
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
