// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package credentials

// StoreID is the id to store the aws credential information
type StoreID string

const (
	// AWSSSMStore is the aws store id shared tests
	AWSSSMStore StoreID = "aws-ssm"
)

// Manager is used for interacting with the aws systems manager for credentials
type Manager interface {
	GetCredential(StoreID, string) (string, error)
}

type manager struct {
	credStores map[StoreID]store
}

// NewManager creates a Manager
func NewManager() Manager {
	return &manager{
		credStores: map[StoreID]store{
			AWSSSMStore: newCachingStore(&awsStore{}),
		},
	}
}

// GetCredential returns a credential for a given storeId and key
func (m *manager) GetCredential(storeID StoreID, key string) (string, error) {
	return m.credStores[storeID].get(key)
}
