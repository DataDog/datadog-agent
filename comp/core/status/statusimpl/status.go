// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"encoding/json"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type dependencies struct {
	fx.In

	Providers []status.StatusProvider `group:"status"`
}

type statusImplementation struct {
	providers []status.StatusProvider
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newStatus),
)

func newStatus(deps dependencies) (status.Component, error) {
	return &statusImplementation{
		providers: deps.Providers,
	}, nil
}

func (s *statusImplementation) Get(format string) ([]byte, error) {
	switch format {
	case "json":
		stats := make(map[string]interface{})
		for _, sc := range s.providers {
			sc.JSON(stats)
		}
		return json.Marshal(stats)
	case "text":
		var b = new(bytes.Buffer)
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
