// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm/credentials"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

// credentialStore reads the SNMP credentials Fleet Automation writes to the
// Agent configuration, which every range references by id.
type credentialStore interface {
	Load() (map[string]credentials.Credential, error)
}

// resolveCredentials maps a range's credential ids to credentials, keeping the
// configured order so the most likely credential is tried first. An unusable
// credential blocks the range instead of failing every chunk of every cycle.
func resolveCredentials(store credentialStore, ids []string) ([]connectivity.SNMPCredential, error) {
	if len(ids) == 0 {
		return nil, errors.New("the range references no credentials")
	}

	available, err := store.Load()
	if err != nil {
		return nil, err
	}

	creds := make([]connectivity.SNMPCredential, 0, len(ids))
	for _, id := range ids {
		c, ok := available[id]
		if !ok {
			return nil, fmt.Errorf("credential %q is not available on this agent", id)
		}
		if err := credentials.Validate(c); err != nil {
			return nil, err
		}
		creds = append(creds, connectivity.SNMPCredential{
			ID:              c.ID,
			Version:         c.SNMPVersion,
			Community:       c.CommunityString,
			User:            c.User,
			AuthProtocol:    c.AuthProtocol,
			AuthKey:         c.AuthKey,
			PrivProtocol:    c.PrivProtocol,
			PrivKey:         c.PrivKey,
			ContextName:     c.ContextName,
			ContextEngineID: c.ContextEngineID,
		})
	}
	return creds, nil
}
