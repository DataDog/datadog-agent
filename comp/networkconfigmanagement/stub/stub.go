// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package stub implements a component that returns a "not available" error for
// every method.
package stub

import (
	"errors"
	"net/http"
	"time"

	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
)

var errNoNCM = errors.New("NCM not available")

// NCMStub is an NCM implementation that returns "not available" for every
// operation.
type NCMStub struct{}

var _ networkconfigmanagement.Component = (*NCMStub)(nil)

func (s *NCMStub) RegisterDevice(_ *config.DeviceInstance) error          { return errNoNCM }
func (s *NCMStub) ReportConfig(_ string) error                            { return errNoNCM }
func (s *NCMStub) ReportConfigWithSender(_ string, _ sender.Sender) error { return errNoNCM }
func (s *NCMStub) RollbackConfig(_, _, _ string) error                    { return errNoNCM }
func (s *NCMStub) SetMaxReportInterval(_ time.Duration)                   {}

// GetConfigEndpointHandler implements [networkconfigmanagement.Component].
func (s *NCMStub) GetConfigEndpointHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error": "ncm rollbacks not available for agent"}`, http.StatusBadRequest)
	}
}
