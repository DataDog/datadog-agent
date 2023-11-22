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

	Providers []status.StatusProvider `group:"status"`
}

type statusImplementation struct {
	headerProvider    headerProvider
	metadataSection   []status.StatusProvider
	collectorSection  []status.StatusProvider
	componentsSection []status.StatusProvider
	providers         []status.StatusProvider
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newStatus),
)

func sortByNameAndSection(section string, providers []status.StatusProvider) []status.StatusProvider {
	var sectionProviders []status.StatusProvider
	for _, provider := range providers {
		if provider.Section() == section {
			sectionProviders = append(sectionProviders, provider)
		}
	}

	sort.SliceStable(sectionProviders, func(i, j int) bool {
		return sectionProviders[i].Name() < sectionProviders[j].Name()
	})

	return sectionProviders
}

func newStatus(deps dependencies) (status.Component, error) {
	metadataSection := sortByNameAndSection("metadata", deps.Providers)
	collectorSection := sortByNameAndSection("collector", deps.Providers)
	componentsSection := sortByNameAndSection("components", deps.Providers)

	return &statusImplementation{
		headerProvider:    newHeaderProvider(deps.Config),
		metadataSection:   metadataSection,
		collectorSection:  collectorSection,
		componentsSection: componentsSection,
		providers:         deps.Providers,
	}, nil
}

func (s *statusImplementation) GetStatus(format string, verbose bool) ([]byte, error) {
	switch format {
	case "json":
		stats := make(map[string]interface{})

		s.headerProvider.JSON(stats)

		for _, sc := range s.providers {
			sc.JSON(stats)
		}
		return json.Marshal(stats)
	case "text":
		var b = new(bytes.Buffer)

		for _, sc := range s.providers {
			sc.AppendToHeader(s.headerProvider.data)
		}

		s.headerProvider.Text(b)

		for _, sc := range s.metadataSection {
			err := sc.Text(b)
			if err != nil {
				return b.Bytes(), err
			}
		}

		for _, sc := range s.collectorSection {
			err := sc.Text(b)
			if err != nil {
				return b.Bytes(), err
			}
		}

		for _, sc := range s.componentsSection {
			err := sc.Text(b)
			if err != nil {
				return b.Bytes(), err
			}
		}

		return b.Bytes(), nil
	case "html":
		var b = new(bytes.Buffer)

		for _, sc := range s.providers {
			sc.AppendToHeader(s.headerProvider.data)
		}

		s.headerProvider.HTML(b)

		for _, sc := range s.metadataSection {
			err := sc.Text(b)
			if err != nil {
				return b.Bytes(), err
			}
		}

		for _, sc := range s.collectorSection {
			err := sc.Text(b)
			if err != nil {
				return b.Bytes(), err
			}
		}

		for _, sc := range s.componentsSection {
			err := sc.Text(b)
			if err != nil {
				return b.Bytes(), err
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

func (s *statusImplementation) GetStatusByNames(names []string, format string, verbose bool) ([]byte, error) {
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

func (s *statusImplementation) GetStatusBySection(section, format string, verbose bool) ([]byte, error) {
	output := func(format string, providers []status.StatusProvider) ([]byte, error) {
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

	switch section {
	case "metadata":
		return output(format, s.metadataSection)
	case "collector":
		return output(format, s.collectorSection)
	case "components":
		return output(format, s.componentsSection)
	default:
		return []byte{}, nil
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
