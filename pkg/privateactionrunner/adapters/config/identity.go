// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

// Identity is the enrolled runner identity used for task verification and OPMS authentication.
type Identity struct {
	OrgId      int64
	PrivateKey *ecdsa.PrivateKey
	RunnerId   string
	Urn        string
}

func (i *Identity) IsIncomplete() bool { return i.Urn == "" || i.PrivateKey == nil }

// ReadIdentity decodes the configured identity. Empty values permit self-enrollment.
func ReadIdentity(config model.Reader) (*Identity, error) {
	encodedPrivateKey := config.GetString(par.PrivateKey)
	urn := config.GetString(par.URN)

	var privateKey *ecdsa.PrivateKey
	if encodedPrivateKey != "" {
		jwk, err := util.Base64ToJWK(encodedPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decode %s: %w", par.PrivateKey, err)
		}
		privateKey = jwk.Key.(*ecdsa.PrivateKey)
	}

	var orgID int64
	var runnerID string
	// allow empty urn for self-enrollment
	if urn != "" {
		urnParts, err := util.ParseRunnerURN(urn)
		if err != nil {
			return nil, fmt.Errorf("failed to parse URN: %w", err)
		}
		orgID = urnParts.OrgID
		runnerID = urnParts.RunnerID
	}

	return &Identity{OrgId: orgID, PrivateKey: privateKey, RunnerId: runnerID, Urn: urn}, nil
}
