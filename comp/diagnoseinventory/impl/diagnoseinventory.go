// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package diagnoseinventoryimpl implements the diagnoseinventory component interface
package diagnoseinventoryimpl

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	diagnoseinventory "github.com/DataDog/datadog-agent/comp/diagnoseinventory/def"
	"github.com/DataDog/datadog-agent/comp/diagnoseinventory/remote"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
)

// Requires defines the dependencies for the diagnoseinventory component
type Requires struct {
	// Remove this field if the component has no lifecycle hooks
	Lifecycle compdef.Lifecycle

	DiagnoseComponent diagnose.Component
	Log               log.Component
	Config            config.Component
	InventoryAgent    inventoryagent.Component
}

// Provides defines the output of the diagnoseinventory component
type Provides struct {
	Comp diagnoseinventory.Component
}

type inventoryImpl struct {
	log               log.Component
	config            config.Component
	inventoryAgent    inventoryagent.Component
	diagnoseComponent diagnose.Component
	stopCh            chan struct{}
}

// NewComponent creates a new diagnoseinventory component
func NewComponent(reqs Requires) (Provides, error) {
	comp := &inventoryImpl{
		log:               reqs.Log,
		config:            reqs.Config,
		inventoryAgent:    reqs.InventoryAgent,
		diagnoseComponent: reqs.DiagnoseComponent,
		stopCh:            make(chan struct{}),
	}

	reqs.Lifecycle.Append(compdef.Hook{OnStart: comp.Start, OnStop: comp.Stop})

	provides := Provides{Comp: comp}
	return provides, nil
}

func (c *inventoryImpl) Start(_ context.Context) error {
	c.stopCh = make(chan struct{})
	go func() {
		for {
			select {
			case <-time.After(10 * time.Minute):
				c.collect()
			case <-c.stopCh:
				return
			}
		}
	}()
	c.collect()
	return nil
}

func (c *inventoryImpl) collect() {
	res, err := remote.Run(diagnose.Config{}, c.diagnoseComponent, c.config)
	if err != nil {
		c.log.Errorf("Error while running diagnostics: %s", err)
		return
	}

	payloads := make([]DiagnosisPayload, 0)

	for _, run := range res.Runs {
		for _, diagnosis := range run.Diagnoses {
			payloads = append(payloads, DiagnosisPayload{
				Status:        diagnosis.Status,
				DiagnosisType: connectivityCheck,
				Name:          diagnosis.Name,
				Error:         diagnosis.RawError,
			})
		}
	}
	c.inventoryAgent.Set("diagnostics", payloads)

}

func (c *inventoryImpl) Stop(_ context.Context) error {
	close(c.stopCh)
	return nil
}

// func marshallPayload(diagnosePayloads []DiagnosisPayload) (string, error) {
// 	diagJSON, err := json.MarshalIndent(diagnosePayloads, "", "  ")
// 	if err != nil {
// 		return "", err
// 	}
// 	return string(diagJSON), nil
// }

// diagnosisType is the type of diagnosis
type diagnosisType int

const (
	// connectivityCheck is the type of diagnosis for connectivity check
	connectivityCheck diagnosisType = iota
)

// DiagnosisPayload contains the result payload of the diagnosis
type DiagnosisPayload struct {
	Status        diagnose.Status `json:"result"`
	DiagnosisType diagnosisType   `json:"diagnosis_type"`
	Name          string          `json:"name"`
	Error         string          `json:"error,omitempty"`
}
