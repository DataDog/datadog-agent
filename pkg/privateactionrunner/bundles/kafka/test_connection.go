// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kafka

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type TestConnectionHandler struct {
	newClient adminClientFactory
}

func NewTestConnectionHandler() *TestConnectionHandler {
	return &TestConnectionHandler{newClient: newAdminClient}
}

type Broker struct {
	NodeID int32  `json:"nodeId"`
	Host   string `json:"host"`
	Port   int32  `json:"port"`
	Rack   string `json:"rack,omitempty"`
}

type ClusterInfo struct {
	ClusterID    string   `json:"clusterId"`
	ControllerID int32    `json:"controllerId"`
	Brokers      []Broker `json:"brokers"`
}

type TestConnectionOutputs struct {
	ConfigurationValid bool         `json:"configurationValid"`
	ConnectionValid    bool         `json:"connectionValid"`
	ClusterInfo        *ClusterInfo `json:"clusterInfo,omitempty"`
	Errors             []string     `json:"errors"`
}

func (h *TestConnectionHandler) Run(ctx context.Context, _ *types.Task, credential *privateconnection.PrivateCredentials) (interface{}, error) {
	client, err := h.newClient(credential)
	if err != nil {
		return &TestConnectionOutputs{
			ConfigurationValid: false,
			ConnectionValid:    false,
			Errors:             []string{err.Error()},
		}, nil
	}
	defer client.Close()

	metadata, err := client.BrokerMetadata(ctx)
	if err != nil {
		return &TestConnectionOutputs{
			ConfigurationValid: true,
			ConnectionValid:    false,
			Errors:             []string{fmt.Sprintf("Unable to connect to Kafka: %v", err)},
		}, nil
	}

	clusterInfo := &ClusterInfo{
		ClusterID:    metadata.Cluster,
		ControllerID: metadata.Controller,
		Brokers:      make([]Broker, 0, len(metadata.Brokers)),
	}
	for _, broker := range metadata.Brokers {
		clusterInfo.Brokers = append(clusterInfo.Brokers, Broker{
			NodeID: broker.NodeID,
			Host:   broker.Host,
			Port:   broker.Port,
			Rack:   derefString(broker.Rack),
		})
	}

	return &TestConnectionOutputs{
		ConfigurationValid: true,
		ConnectionValid:    true,
		ClusterInfo:        clusterInfo,
	}, nil
}
