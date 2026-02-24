// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package delegatedauthimpl provides a no-op implementation of the delegatedauth component
package delegatedauthimpl

import (
	"io"

	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
)

// Provides list the provided interfaces from the delegatedauth Component
type Provides struct {
	Comp           delegatedauth.Component
	StatusProvider status.InformationProvider
}

type delegatedAuthNoop struct{}

var _ delegatedauth.Component = (*delegatedAuthNoop)(nil)

// NewComponent returns a no-op implementation for the delegated auth component
func NewComponent() Provides {
	noop := &delegatedAuthNoop{}
	// Note: importing log package would require adding it as a dependency, so skipping debug here
	return Provides{
		Comp:           noop,
		StatusProvider: status.NewInformationProvider(noop),
	}
}

// AddInstance does nothing in the noop implementation
func (d *delegatedAuthNoop) AddInstance(_ delegatedauth.InstanceParams) error {
	return nil
}

// Status Provider implementation for noop

// Name returns the name for status sorting
func (d *delegatedAuthNoop) Name() string {
	return "Delegated Auth"
}

// Section returns the section name for status grouping
func (d *delegatedAuthNoop) Section() string {
	return "delegatedauth"
}

// JSON populates the status stats map
func (d *delegatedAuthNoop) JSON(_ bool, stats map[string]interface{}) error {
	stats["enabled"] = false
	return nil
}

// Text renders the text status output
func (d *delegatedAuthNoop) Text(_ bool, buffer io.Writer) error {
	_, err := buffer.Write([]byte("Delegated Authentication is not enabled\n"))
	return err
}

// HTML renders the HTML status output
func (d *delegatedAuthNoop) HTML(_ bool, buffer io.Writer) error {
	_, err := buffer.Write([]byte("<div class=\"stat\"><span class=\"stat_title\">Delegated Authentication</span><span class=\"stat_data\">Not enabled</span></div>\n"))
	return err
}
