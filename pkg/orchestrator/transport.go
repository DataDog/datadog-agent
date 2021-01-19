// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package orchestrator

import (
	"net/http"
	"strconv"
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/process/util/api"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Sender exposes a way to send orchestrator payload
type Sender struct {
	hostName  string
	forwarder forwarder.Forwarder
}

// Start starts the sender's forwarder
func (s *Sender) Start() error {
	if err := s.forwarder.Start(); err != nil {
		log.Errorf("Error starting manifest forwarder: %s", err)
		return err
	}
	return nil
}

// Stop stops the sender's forwarder
func (s *Sender) Stop() {
	s.forwarder.Stop()
}

// NewSender returns a new sender
func NewSender(cfg *config.OrchestratorConfig, hostname string) *Sender {
	keysPerDomain := api.KeysPerDomains(cfg.OrchestratorEndpoints)
	orchestratorForwarderOpts := forwarder.NewOptions(keysPerDomain)
	orchestratorForwarderOpts.DisableAPIKeyChecking = true

	return &Sender{
		hostName:  hostname,
		forwarder: forwarder.NewDefaultForwarder(orchestratorForwarderOpts),
	}
}

// SendMessages sends a message to the orchestrator intake
func (s *Sender) SendMessages(msg []model.MessageBody, payloadType string) {
	clusterID, err := clustername.GetClusterID()
	if err != nil {
		log.Errorf("Could not send manifest messages: %s", err)
		return
	}

	for _, m := range msg {
		extraHeaders := make(http.Header)
		extraHeaders.Set(api.HostHeader, s.hostName)
		extraHeaders.Set(api.ClusterIDHeader, clusterID)
		extraHeaders.Set(api.TimestampHeader, strconv.Itoa(int(time.Now().Unix())))

		body, err := api.EncodePayload(m)
		if err != nil {
			log.Errorf("Unable to encode message: %s", err)
			continue
		}

		payloads := forwarder.Payloads{&body}
		responses, err := s.forwarder.SubmitOrchestratorChecks(payloads, extraHeaders, payloadType)
		if err != nil {
			log.Errorf("Unable to submit payload: %s", err)
			continue
		}

		// Consume the responses so that writers to the channel do not become blocked
		// we don't need the bodies here though
		for range responses {

		}
	}
}
