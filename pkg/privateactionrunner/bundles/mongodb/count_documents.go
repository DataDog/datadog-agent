// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_mongodb

import (
	"context"
	"fmt"

	credssupport "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/credentials"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"go.mongodb.org/mongo-driver/mongo"
)

type CountDocumentsAction struct{}

func NewCountDocumentsHandler() types.Action {
	return &CountDocumentsAction{}
}

type CountDocumentsInputs struct {
	Collection string         `json:"collection,omitempty"`
	Filter     map[string]any `json:"filter,omitempty"`
}

type CountDocumentsOutputs struct {
	Count int64 `json:"count"`
}

func (cda CountDocumentsAction) Run(ctx context.Context, task *types.Task, credential interface{}) (interface{}, error) {
	inputs, err := types.ExtractInputs[CountDocumentsInputs](task)
	if err != nil {
		return nil, fmt.Errorf("Failed to extract inputs: %w", err)
	}

	if err := ValidateFilter(inputs.Filter); err != nil {
		return nil, err
	}

	credentialTokens, err := credssupport.ToTokensMap(credential)
	if err != nil {
		return nil, fmt.Errorf("Failed to convert credentials: %w", err)
	}

	clientOptions, cs, err := createMongoClientOptions(ctx, credentialTokens)
	if err != nil {
		return nil, err
	}

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("Unable to connect to MongoDB: %w", err)
	}
	defer func() {
		if err := client.Disconnect(ctx); err != nil {
			log.Errorf("Error disconnecting from MongoDB %w", err)
		}
	}()

	db := client.Database(cs.Database)
	collection := db.Collection(inputs.Collection)

	count, err := collection.CountDocuments(ctx, inputs.Filter)
	if err != nil {
		return nil, fmt.Errorf("Failed to count documents: %w", err)
	}

	return &CountDocumentsOutputs{
		Count: count,
	}, nil
}
