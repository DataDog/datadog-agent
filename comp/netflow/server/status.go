// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package server

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
	nfconfig "github.com/DataDog/datadog-agent/comp/netflow/config"
)

//go:embed status_templates
var templatesFS embed.FS

// netflowServerStatus represents the status of the server including details about
// listeners which are working and those which have closed.
type netflowServerStatus struct {
	TotalListeners         int
	OpenListeners          int
	ClosedListeners        int
	WorkingListenerDetails []netflowListenerStatus
	ClosedListenerDetails  []netflowListenerStatus
}

// netflowListenerStatus handles logic related to pulling config information and associating it to an error.
type netflowListenerStatus struct {
	Config    nfconfig.ListenerConfig
	Error     string
	FlowCount int64
}

// Provider provides the functionality to populate the status output
type Provider struct {
	server *Server
}

// Name returns the name
func (Provider) Name() string {
	return "NetFlow"
}

// Section return the section
func (Provider) Section() string {
	return "NetFlow"
}

// JSON populates the status map
func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	p.getStatus(stats)

	return nil
}

// Text renders the text output
func (p Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "netflow.tmpl", buffer, p.populateStatus())
}

// HTML renders the html output
func (p Provider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "netflowHTML.tmpl", buffer, p.populateStatus())
}

func (p Provider) getStatus(stats map[string]interface{}) {
	workingListeners := []netflowListenerStatus{}
	closedListenersList := []netflowListenerStatus{}

	for _, listener := range p.server.listeners {
		errorString := listener.error.Load()
		if errorString != "" {
			closedListenersList = append(closedListenersList, netflowListenerStatus{
				Config: listener.config,
				Error:  errorString,
			})
		} else {
			workingListeners = append(workingListeners, netflowListenerStatus{
				Config:    listener.config,
				FlowCount: listener.flowCount.Load(),
			})
		}
	}

	status := netflowServerStatus{
		TotalListeners:         int(len(p.server.listeners)),
		OpenListeners:          int(len(workingListeners)),
		ClosedListeners:        int(len(closedListenersList)),
		WorkingListenerDetails: workingListeners,
		ClosedListenerDetails:  closedListenersList,
	}

	stats["netflowStats"] = status
}

func (p Provider) populateStatus() map[string]interface{} {
	stats := make(map[string]interface{})

	p.getStatus(stats)

	return stats
}
