package com_datadoghq_mongodb

import (
	"context"
	"fmt"

	credssupport "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/credentials"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ReplaceOneAction struct{}

func NewReplaceOneHandler() types.Action {
	return &ReplaceOneAction{}
}

type ReplaceOneInputs struct {
	Collection  string         `json:"collection,omitempty"`
	Filter      map[string]any `json:"filter,omitempty"`
	Replacement map[string]any `json:"replacement,omitempty"`
	Upsert      bool           `json:"upsert,omitempty"`
}

type ReplaceOneOutputs struct {
	MatchedCount  int64       `json:"matchedCount"`
	ModifiedCount int64       `json:"modifiedCount"`
	UpsertedID    interface{} `json:"upsertedId,omitempty"`
}

func (ro ReplaceOneAction) Run(ctx context.Context, task *types.Task, credential interface{}) (interface{}, error) {
	inputs, err := types.ExtractInputs[ReplaceOneInputs](task)
	if err != nil {
		return nil, fmt.Errorf("failed to extract inputs: %w", err)
	}

	if err := ValidateFilter(inputs.Filter); err != nil {
		return nil, err
	}

	credentialTokens, err := credssupport.ToTokensMap(credential)
	if err != nil {
		return nil, fmt.Errorf("failed to convert credentials: %w", err)
	}

	clientOptions, cs, err := createMongoClientOptions(ctx, credentialTokens)
	if err != nil {
		return nil, err
	}

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to MongoDB: %w", err)
	}
	defer func() {
		if err := client.Disconnect(ctx); err != nil {
			log.Error("Error disconnecting from MongoDB %w", err)
		}
	}()

	db := client.Database(cs.Database)
	collection := db.Collection(inputs.Collection)

	replaceOptions := options.Replace().SetUpsert(inputs.Upsert)
	result, err := collection.ReplaceOne(ctx, inputs.Filter, inputs.Replacement, replaceOptions)
	if err != nil {
		return nil, fmt.Errorf("Failed to replace document: %w", err)
	}

	var upsertedID string
	if result.UpsertedID != nil {
		objectID, ok := result.UpsertedID.(primitive.ObjectID)
		if !ok {
			return nil, fmt.Errorf("Failed to retrieve upserted ID")
		}
		upsertedID = objectID.Hex()
	}

	return &ReplaceOneOutputs{
		MatchedCount:  result.MatchedCount,
		ModifiedCount: result.ModifiedCount,
		UpsertedID:    upsertedID,
	}, nil
}
