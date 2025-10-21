// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flareimpl implements the flareimpl
package flareimpl

import (
	"encoding/json"
	"fmt"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	hpflareextension "github.com/DataDog/datadog-agent/comp/host-profiler/hpflareextension/impl"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

// Provides specifics the types returned by the constructor
type Provides struct {
	FlareProvider flaretypes.Provider
}

// Requires defines the dependencies for the flareimpl component
type Requires struct {
	Client ipc.HTTPClient
}

type flareImpl struct {
	client ipc.HTTPClient
}

// NewComponent creates a new Component for this module and returns any errors on failure.
func NewComponent(reqs Requires) (Provides, error) {
	flare := flareImpl{
		client: reqs.Client,
	}
	return Provides{
		FlareProvider: flaretypes.NewProvider(flare.fillFlare),
	}, nil
}

func (c *flareImpl) fillFlare(fb flaretypes.FlareBuilder) error {
	responseBytes, err := c.requestOtelConfigInfo()
	if err != nil {
		msg := fmt.Sprintf("did not get host-profiler configuration: %v", err)
		log.Error(msg)
		fb.AddFile("host-profiler/host-profiler.log", []byte(msg))

		return nil
	}

	var responseInfo hpflareextension.Response
	if err := json.Unmarshal(responseBytes, &responseInfo); err != nil {
		msg := fmt.Sprintf("could not read sources from host-profiler response: %s, error: %v", responseBytes, err)
		log.Error(msg)
		fb.AddFile("host-profiler/host-profiler.log", []byte(msg))
		return nil
	}

	fb.AddFile("host-profiler/runtime.cfg", []byte(toJSON(responseInfo.Config)))
	return nil
}

func toJSON(it interface{}) string {
	data, err := json.Marshal(it)
	if err != nil {
		return err.Error()
	}
	return string(data)
}

func (c *flareImpl) requestOtelConfigInfo() ([]byte, error) {
	// todo(mackjmr): Make port configurable once we have agreement on hostprofiler config.
	data, err := c.client.Get("https://localhost:7778")
	if err != nil {
		return nil, err
	}

	return data, nil
}
