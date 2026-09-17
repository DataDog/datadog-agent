// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package enrollment

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	configModel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func encodeIdentity(result *Result) ([]byte, error) {
	key, err := util.EcdsaToJWK(result.PrivateKey)
	if err != nil {
		return nil, err
	}
	raw, err := key.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return json.Marshal(PersistedIdentity{AuthorizationVersion: result.AuthorizationVersion, PrivateKey: base64.RawURLEncoding.EncodeToString(raw), URN: result.URN, Hostname: result.Hostname, OrchClusterID: result.OrchClusterID, APIKeyHash: result.APIKeyHash, AuthorizationType: result.AuthorizationType, IntakeMappingID: result.IntakeMappingID, Provider: result.Provider, Pending: result.Pending, RunnerName: result.RunnerName})
}

// writeIdentityFile writes a complete mode-0600 file before making it visible.
// Exclusive publication claims the pending key without overwriting another process.
func writeIdentityFile(path string, data []byte, exclusive bool) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".par-identity-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if exclusive {
		err = os.Link(tmp, path)
	} else {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		dir, err := os.Open(filepath.Dir(path))
		if err != nil {
			return err
		}
		defer dir.Close()
		return dir.Sync()
	}
	return nil
}

func claimPendingIdentity(ctx context.Context, cfg configModel.Reader, candidate *Result) (*PersistedIdentity, error) {
	if cfg.GetBool(setup.PARIdentityUseK8sSecret) && flavor.GetFlavor() == flavor.ClusterAgent {
		return claimPendingK8sIdentity(ctx, cfg, candidate)
	}
	raw, err := encodeIdentity(candidate)
	if err != nil {
		return nil, err
	}
	err = writeIdentityFile(getIdentityFilePath(cfg), raw, true)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	return getIdentityFromFile(cfg)
}
func canManageWIFIdentity(cfg configModel.Reader) (bool, error) {
	if flavor.GetFlavor() != flavor.ClusterAgent {
		return true, nil
	}
	if !cfg.GetBool(setup.PARIdentityUseK8sSecret) {
		return false, errors.New("workload enrollment on Cluster Agent requires shared Kubernetes identity storage")
	}
	return isWIFLeader()
}
