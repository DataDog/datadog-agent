// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package sign holds event signature related files
package sign

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math/rand"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

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

// Sign a process cache entry and returns the result
func (r *Resolver) Sign(pce *model.ProcessContext) (string, error) {
	if pce == nil {
		return "", errors.New("no valid process cache entry supplied")
	}

	h := sha256.New()
	h.Write([]byte(pce.Process.CGroup.CGroupID))
	if err := binary.Write(h, binary.LittleEndian, r.signatureKey); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
