// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"encoding/json"

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
	headerProvider headerProvider
	providers      []status.StatusProvider
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newStatus),
)

func newStatus(deps dependencies) (status.Component, error) {
	// TODO: sort providers by index and name
	return &statusImplementation{
		headerProvider: newHeaderProvider(deps.Config),
		providers:      deps.Providers,
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

		for _, sc := range s.providers {
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

		for _, sc := range s.providers {
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

func include(value string, container []string) bool {
	for _, v := range container {
		if v == value {
			return true
		}
	}

	return false
}
