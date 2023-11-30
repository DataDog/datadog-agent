// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"encoding/json"
	"sort"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type dependencies struct {
	fx.In
	Config config.Component

	Providers       []status.StatusProvider       `group:"status"`
	HeaderProviders []status.HeaderStatusProvider `group:"header_status"`
}

type statusImplementation struct {
	sortedHeaderSection []status.HeaderStatusProvider
	sortedSections      map[string][]status.StatusProvider
	providers           []status.StatusProvider
	headerProvider      []status.HeaderStatusProvider
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newStatus),
)

func sortByName(providers []status.StatusProvider) []status.StatusProvider {
	sort.SliceStable(providers, func(i, j int) bool {
		return providers[i].Name() < providers[j].Name()
	})

	return providers
}

func newStatus(deps dependencies) (status.Component, error) {
	sortedSections := map[string][]status.StatusProvider{}
	for _, provider := range deps.Providers {
		providers := sortedSections[provider.Section()]
		sortedSections[provider.Section()] = append(providers, provider)
	}

	for section, providers := range sortedSections {
		sortedSections[section] = sortByName(providers)
	}

	sortedHeaderSection := deps.HeaderProviders
	sort.SliceStable(sortedHeaderSection, func(i, j int) bool {
		return sortedHeaderSection[i].Index() < sortedHeaderSection[j].Index()
	})

	sortedHeaderSection = append([]status.HeaderStatusProvider{newHeaderProvider(deps.Config)}, sortedHeaderSection...)

	return &statusImplementation{
		sortedSections:      sortedSections,
		sortedHeaderSection: sortedHeaderSection,
		providers:           deps.Providers,
		headerProvider:      deps.HeaderProviders,
	}, nil
}

func (s *statusImplementation) GetStatus(format string, verbose bool) ([]byte, error) {
	switch format {
	case "json":
		stats := make(map[string]interface{})
		for _, sc := range s.sortedHeaderSection {
			sc.JSON(stats)
		}

		for _, providers := range s.sortedSections {
			for _, provider := range providers {
				provider.JSON(stats)
			}
		}
		return json.Marshal(stats)
	case "text":
		var b = new(bytes.Buffer)

		for _, sc := range s.sortedHeaderSection {
			err := sc.Text(b)
			if err != nil {
				return b.Bytes(), err
			}
		}

		for _, sc := range s.sortedSections[status.CollectorSection] {
			err := sc.Text(b)
			if err != nil {
				return b.Bytes(), err
			}
		}

		for section, providers := range s.sortedSections {
			if section == status.CollectorSection {
				continue
			}

			// TODO: Print section header
			for _, provider := range providers {
				err := provider.Text(b)
				if err != nil {
					return b.Bytes(), err
				}
			}
		}
		return b.Bytes(), nil
	case "html":
		var b = new(bytes.Buffer)

		for _, sc := range s.sortedHeaderSection {
			err := sc.HTML(b)
			if err != nil {
				return b.Bytes(), err
			}
		}

		for _, sc := range s.sortedSections[status.CollectorSection] {
			err := sc.HTML(b)
			if err != nil {
				return b.Bytes(), err
			}
		}

		for section, providers := range s.sortedSections {
			if section == status.CollectorSection {
				continue
			}

			for _, provider := range providers {
				err := provider.HTML(b)
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

func (s *statusImplementation) GetStatusByName(name, format string, verbose bool) ([]byte, error) {
	var statusSectionProvider status.StatusProvider
	for _, provider := range s.providers {
		if provider.Name() == name {
			statusSectionProvider = provider
			break
		}
	}

	switch format {
	case "json":
		stats := make(map[string]interface{})

		statusSectionProvider.JSON(stats)
		return json.Marshal(stats)
	case "text":
		var b = new(bytes.Buffer)
		err := statusSectionProvider.Text(b)
		if err != nil {
			return b.Bytes(), err
		}
		return b.Bytes(), nil
	case "html":
		var b = new(bytes.Buffer)
		err := statusSectionProvider.HTML(b)
		if err != nil {
			return b.Bytes(), err
		}
		return b.Bytes(), nil
	default:
		return []byte{}, nil
	}
}

func (s *statusImplementation) GetStatusesByName(names []string, format string, verbose bool) ([]byte, error) {
	var providers []status.StatusProvider
	for _, provider := range s.providers {
		if include(provider.Name(), names) {
			providers = append(providers, provider)
		}
	}

	switch format {
	case "json":
		stats := make(map[string]interface{})

		for _, sp := range providers {
			sp.JSON(stats)
		}
		return json.Marshal(stats)
	case "text":
		var b = new(bytes.Buffer)

		for _, sp := range providers {
			err := sp.Text(b)
			if err != nil {
				return b.Bytes(), err
			}
		}

		return b.Bytes(), nil
	case "html":
		var b = new(bytes.Buffer)

		for _, sp := range providers {
			err := sp.HTML(b)
			if err != nil {
				return b.Bytes(), err
			}
		}
		return b.Bytes(), nil
	default:
		return []byte{}, nil
	}
}

func (s *statusImplementation) GetStatusBySection(section string, format string, verbose bool) ([]byte, error) {
	switch section {
	case "header":
		providers := s.sortedHeaderSection
		switch format {
		case "json":
			stats := make(map[string]interface{})

			for _, sc := range providers {
				sc.JSON(stats)
			}
			return json.Marshal(stats)
		case "text":
			var b = new(bytes.Buffer)

			for _, sc := range providers {
				err := sc.Text(b)
				if err != nil {
					return b.Bytes(), err
				}
			}

			return b.Bytes(), nil
		case "html":
			var b = new(bytes.Buffer)

			for _, sc := range providers {
				err := sc.HTML(b)
				if err != nil {
					return b.Bytes(), err
				}
			}
			return b.Bytes(), nil
		default:
			return []byte{}, nil
		}
	default:
		providers := s.sortedSections[section]
		switch format {
		case "json":
			stats := make(map[string]interface{})

			for _, sc := range providers {
				sc.JSON(stats)
			}
			return json.Marshal(stats)
		case "text":
			var b = new(bytes.Buffer)

			for _, sc := range providers {
				err := sc.Text(b)
				if err != nil {
					return b.Bytes(), err
				}
			}

			return b.Bytes(), nil
		case "html":
			var b = new(bytes.Buffer)

			for _, sc := range providers {
				err := sc.HTML(b)
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

func include(value string, container []string) bool {
	for _, v := range container {
		if v == value {
			return true
		}
	}

	return false
}
