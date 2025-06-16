// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package inventoryimpl implements a component to collect diagnose inventory.
package inventoryimpl

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	inventory "github.com/DataDog/datadog-agent/comp/core/diagnose/inventory/def"
	"github.com/DataDog/datadog-agent/comp/core/diagnose/remote"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
)

type dependencies struct {
	fx.In

	Log            log.Component
	Config         config.Component
	InventoryAgent inventoryagent.Component
}

// Provides defines the output of the diagnose inventorycomponent
type Provides struct {
	fx.Out

	Comp inventory.Component
}

type inventoryImpl struct {
	log            log.Component
	config         config.Component
	inventoryAgent inventoryagent.Component
	stopCh         chan struct{}
}

// NewComponent creates a new diagnose inventory component
func NewComponent(deps dependencies) Provides {
	comp := &inventoryImpl{
		log:            deps.Log,
		config:         deps.Config,
		inventoryAgent: deps.InventoryAgent,
		stopCh:         make(chan struct{}),
	}

	return Provides{
		Comp: comp,
	}
}

func (comp *inventoryImpl) Start(_ context.Context) error {
	go func() {
		for {
			select {
			case <-time.After(10 * time.Minute):
				comp.Collect()
			case <-comp.stopCh:
				return
			}
		}
	}()
	comp.Collect()
	return nil
}

func (comp *inventoryImpl) Stop(_ context.Context) error {
	close(comp.stopCh)
	return nil
}

func (comp *inventoryImpl) Collect() {
	res := remote.Run(diagnose.Config{}, comp.config)

	payload, err := formatPayload(res)
	if err != nil {
		comp.log.Errorf("Error while formatting diagnose payload: %s", err)
	}
	comp.inventoryAgent.Set("diagnoses", payload)
}

func formatPayload(diagnosePayloads []remote.DiagnosisPayload) ([]byte, error) {
	var buffer bytes.Buffer
	var err error
	writer := bufio.NewWriter(&buffer)

	diagJSON, err := json.MarshalIndent(diagnosePayloads, "", "  ")
	if err != nil {
		body, _ := json.Marshal(map[string]string{"error": fmt.Sprintf("marshalling diagnose results to JSON: %s", err)})
		fmt.Fprintln(writer, string(body))
		return nil, err
	}
	fmt.Fprintln(writer, string(diagJSON))
	writer.Flush()

	return buffer.Bytes(), nil
}
