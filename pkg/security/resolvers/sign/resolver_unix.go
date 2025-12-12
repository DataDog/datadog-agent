// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package sign holds event signature related files
package sign

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/rand"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

type eventContextToSign struct {
	CgroupID string `json:"cgroupID"`
	Pid      uint32 `json:"pid"`
	Key      uint64 `json:"key"`
}

type Resolver struct {
	signatureKey uint64
}

var newSignatureResolver = sync.OnceValue(func() *Resolver {
	rand.Seed(time.Now().UnixNano())
	return &Resolver{
		signatureKey: rand.Uint64(),
	}
})

func NewSignatureResolver() *Resolver {
	return newSignatureResolver()
}

// Sign a process cach entry and returns the result
func (r *Resolver) Sign(pce *model.ProcessContext) (string, error) {
	if pce == nil {
		return "", errors.New("no valid process cache entry supplied")
	}
	ctx := eventContextToSign{
		CgroupID: string(pce.Process.CGroup.CGroupID),
		Pid:      pce.Process.Pid,
		Key:      r.signatureKey,
	}

	b, err := json.Marshal(ctx)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:]), nil
}
