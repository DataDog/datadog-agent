// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package statusimpl implements the status component interface
package statusimpl

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// httpClients should be reused instead of created as needed. They keep cached TCP connections
// that may leak otherwise
var (
	httpClient     *http.Client
	clientInitOnce sync.Once
)

func client() *http.Client {
	clientInitOnce.Do(func() {
		httpClient = apiutil.GetClient(false)
	})

	return httpClient
}

type provides struct {
	fx.Out

	StatusProvider status.InformationProvider
}

// Module defines the fx options for the status component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newStatus))
}

type statusProvider struct {
	testServerURL string
}

func newStatus() provides {
	return provides{
		StatusProvider: status.NewInformationProvider(statusProvider{}),
	}
}

//go:embed status_templates
var templatesFS embed.FS

// Name returns the name
func (s statusProvider) Name() string {
	return "Process Agent"
}

// Section return the section
func (s statusProvider) Section() string {
	return "Process Agent"
}

func (s statusProvider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	values := s.populateStatus()

	stats["processAgentStatus"] = values

	return stats
}

func (s statusProvider) populateStatus() map[string]interface{} {
	status := make(map[string]interface{})

	c := client()

	var url string
	if s.testServerURL != "" {
		url = s.testServerURL
	} else {
		addressPort, err := config.GetProcessAPIAddressPort()
		if err != nil {
			status["error"] = fmt.Sprintf("%v", err.Error())
			return status
		}
		url = fmt.Sprintf("http://%s/agent/status", addressPort)
	}

	b, err := apiutil.DoGet(c, url, apiutil.CloseConnection)

	if err != nil {
		status["error"] = fmt.Sprintf("%v", err.Error())
		return status
	}

	err = json.Unmarshal(b, &status)
	if err != nil {
		return map[string]interface{}{
			"error": fmt.Sprintf("%v", err.Error()),
		}
	}

	return status
}

// JSON populates the status map
func (s statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	values := s.populateStatus()

	stats["processAgentStatus"] = values

	return nil
}

// Text renders the text output
func (s statusProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "processagent.tmpl", buffer, s.getStatusInfo())
}

// HTML renders the html output
func (s statusProvider) HTML(_ bool, _ io.Writer) error {
	return nil
}
