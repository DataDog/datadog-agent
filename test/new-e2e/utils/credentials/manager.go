// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package credentials

type StoreID string

const (
	AWSSSMStore StoreID = "aws-ssm"
)

type Manager interface {
	GetCredential(StoreID, string) (string, error)
}

type manager struct {
	credStores map[StoreID]store
}

func NewManager() Manager {
	return &manager{
		credStores: map[StoreID]store{
			AWSSSMStore: newCachingStore(&awsStore{}),
		},
	}
}

func (m *manager) GetCredential(storeID StoreID, key string) (string, error) {
	return m.credStores[storeID].get(key)
}
