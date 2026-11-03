// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privilegedhelper

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// DirectorKeyProof contains the TUF metadata needed to authenticate one
// AP_RUNNER_KEYS target from the Director root in the helper credential.
type DirectorKeyProof struct {
	Roots      [][]byte `json:"roots,omitempty"`
	Targets    []byte   `json:"targets"`
	TargetPath string   `json:"targetPath"`
	TargetFile []byte   `json:"targetFile"`
}

type remoteConfigKey struct {
	KeyType KeyType `json:"keyType"`
	Key     []byte  `json:"key"`
}

func validateDirectorRoot(root json.RawMessage) error {
	if _, err := state.NewRepository(root); err != nil {
		return fmt.Errorf("initialize Director repository: %w", err)
	}
	return nil
}

func (c *Credential) withDirectorProof(material CredentialKey) (*Credential, error) {
	if material.Type != KeyTypeTUFDirector {
		return nil, fmt.Errorf("socket verification material must be %q", KeyTypeTUFDirector)
	}
	if len(c.DirectorRoot) == 0 {
		return nil, errors.New("helper credential requires directorRoot")
	}

	var proof DirectorKeyProof
	decoder := json.NewDecoder(strings.NewReader(material.PEM))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proof); err != nil {
		return nil, fmt.Errorf("decode Director proof: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, fmt.Errorf("decode Director proof: %w", err)
	}
	if material.ID == "" || proof.TargetPath == "" || material.ID != proof.TargetPath {
		return nil, errors.New("Director proof target path does not match verification material")
	}
	if err := validateActionPlatformKeyPath(proof.TargetPath, c.OrgID); err != nil {
		return nil, err
	}
	if len(proof.Targets) == 0 || len(proof.TargetFile) == 0 {
		return nil, errors.New("Director proof requires Targets metadata and target file")
	}

	repository, err := state.NewRepository(c.DirectorRoot)
	if err != nil {
		return nil, fmt.Errorf("initialize Director repository: %w", err)
	}
	if _, err := repository.Update(state.Update{
		TUFRoots:      proof.Roots,
		TUFTargets:    proof.Targets,
		TargetFiles:   map[string][]byte{proof.TargetPath: proof.TargetFile},
		ClientConfigs: []string{proof.TargetPath},
	}); err != nil {
		return nil, fmt.Errorf("verify Director proof: %w", err)
	}

	config, ok := repository.GetConfigs(state.ProductActionPlatformRunnerKeys)[proof.TargetPath]
	if !ok {
		return nil, errors.New("Director proof did not contain an AP_RUNNER_KEYS target")
	}
	var remoteKey remoteConfigKey
	decoder = json.NewDecoder(strings.NewReader(string(config.Config)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&remoteKey); err != nil {
		return nil, fmt.Errorf("decode Director-authenticated key: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, fmt.Errorf("decode Director-authenticated key: %w", err)
	}
	if len(remoteKey.Key) == 0 {
		return nil, errors.New("Director-authenticated key is empty")
	}

	credential := *c
	credential.decodedKeys = make(map[string]verificationKey, len(c.decodedKeys)+1)
	for id, key := range c.decodedKeys {
		credential.decodedKeys[id] = key
	}
	if err := addCredentialKey(credential.decodedKeys, CredentialKey{
		ID:   config.Metadata.ID,
		Type: remoteKey.KeyType,
		PEM:  string(remoteKey.Key),
	}, true); err != nil {
		return nil, err
	}
	return &credential, nil
}

func validateActionPlatformKeyPath(targetPath string, orgID int64) error {
	parts := strings.Split(targetPath, "/")
	if len(parts) != 5 || parts[0] != "datadog" ||
		parts[2] != state.ProductActionPlatformRunnerKeys ||
		parts[3] == "" || parts[4] == "" {
		return fmt.Errorf("invalid AP_RUNNER_KEYS target path %q", targetPath)
	}
	pathOrgID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || pathOrgID != orgID {
		return errors.New("AP_RUNNER_KEYS target org does not match helper credential")
	}
	return nil
}
