// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_mongodb

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type TestConnectionHandler struct{}

func NewTestConnectionHandler() *TestConnectionHandler {
	return &TestConnectionHandler{}
}

type TestConnectionInputs struct{}

type ServerInfo struct {
	Version string `json:"version"`
}

type TestConnectionOutputs struct {
	ConfigurationValid bool        `json:"configurationValid"`
	ConnectionValid    bool        `json:"connectionValid"`
	ServerInfo         *ServerInfo `json:"serverInfo,omitempty"`
	Errors             []string    `json:"errors"`
}

func (h *TestConnectionHandler) Run(ctx context.Context, task *types.Task, credential *privateconnection.PrivateCredentials) (interface{}, error) {
	credentialTokens := credential.AsTokenMap()

	clientOptions, _, err := createMongoClientOptions(ctx, credentialTokens)
	if err != nil {
		return &TestConnectionOutputs{
			ConfigurationValid: false,
			ConnectionValid:    false,
			Errors:             []string{err.Error()},
		}, nil
	}

	client, err := mongo.Connect(clientOptions)
	if err != nil {
		return &TestConnectionOutputs{
			ConfigurationValid: true,
			ConnectionValid:    false,
			Errors:             []string{fmt.Sprintf("Unable to connect to MongoDB: %v", err)},
		}, nil
	}
	defer func() {
		if err := client.Disconnect(ctx); err != nil {
			log.FromContext(ctx).Error("Error disconnecting from MongoDB", log.ErrorField(err))
		}
	}()

	if err := client.Ping(ctx, nil); err != nil {
		return &TestConnectionOutputs{
			ConfigurationValid: true,
			ConnectionValid:    false,
			Errors:             []string{fmt.Sprintf("Failed to ping MongoDB server: %v", err)},
		}, nil
	}

	var errors []string
	var serverInfo *ServerInfo
	var buildInfo struct {
		Version string `bson:"version"`
	}
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&buildInfo); err != nil {
		errors = append(errors, fmt.Sprintf("Warning: Failed to get server version: %v", err))
	} else {
		serverInfo = &ServerInfo{Version: buildInfo.Version}
	}

	return &TestConnectionOutputs{
		ConfigurationValid: true,
		ConnectionValid:    true,
		ServerInfo:         serverInfo,
		Errors:             errors,
	}, nil
}
