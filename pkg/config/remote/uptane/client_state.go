// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package uptane

// State represents the state of an uptane client
type State struct {
	ConfigState     map[string]MetaState
	DirectorState   map[string]MetaState
	TargetFilenames map[string]string
}

// MetaState represents the state of a tuf file
type MetaState struct {
	Version uint64
	Hash    string
}

// ConfigRootVersion returns the version of the config root.json file
func (s *State) ConfigRootVersion() uint64 {
	meta, found := s.ConfigState[metaRoot]
	if !found {
		return 0
	}
	return meta.Version
}

// ConfigSnapshotVersion returns the version of the config snapshot.json file
func (s *State) ConfigSnapshotVersion() uint64 {
	meta, found := s.ConfigState[metaSnapshot]
	if !found {
		return 0
	}
	return meta.Version
}

// DirectorRootVersion returns the version of the director root.json file
func (s *State) DirectorRootVersion() uint64 {
	meta, found := s.DirectorState[metaRoot]
	if !found {
		return 0
	}
	return meta.Version
}

// DirectorTargetsVersion returns the version of the director targets.json file
func (s *State) DirectorTargetsVersion() uint64 {
	meta, found := s.DirectorState[metaTargets]
	if !found {
		return 0
	}
	return meta.Version
}

// TUFVersions TODO <remote-config>
type TUFVersions struct {
	DirectorRoot    uint64
	DirectorTargets uint64
	ConfigRoot      uint64
	ConfigSnapshot  uint64
}

// TUFVersionState TODO <remote-config>
func (c *Client) TUFVersionState() (TUFVersions, error) {
	c.Lock()
	defer c.Unlock()

	drv, err := c.directorLocalStore.GetMetaVersion(metaRoot)
	if err != nil {
		return TUFVersions{}, err
	}

	dtv, err := c.directorLocalStore.GetMetaVersion(metaTargets)
	if err != nil {
		return TUFVersions{}, err
	}

	crv, err := c.configLocalStore.GetMetaVersion(metaRoot)
	if err != nil {
		return TUFVersions{}, err
	}

	csv, err := c.configLocalStore.GetMetaVersion(metaSnapshot)
	if err != nil {
		return TUFVersions{}, err
	}

	return TUFVersions{
		DirectorRoot:    drv,
		DirectorTargets: dtv,
		ConfigRoot:      crv,
		ConfigSnapshot:  csv,
	}, nil
}

// State returns the state of the uptane client
func (c *Client) State() (State, error) {
	c.Lock()
	defer c.Unlock()

	s := State{
		ConfigState:     map[string]MetaState{},
		DirectorState:   map[string]MetaState{},
		TargetFilenames: map[string]string{},
	}

	metas, err := c.configLocalStore.GetMeta()
	if err != nil {
		return State{}, err
	}

	for metaName, content := range metas {
		version, err := metaVersion(content)
		if err == nil {
			s.ConfigState[metaName] = MetaState{Version: version, Hash: metaHash(content)}
		}
	}

	directorMetas, err := c.directorLocalStore.GetMeta()
	if err != nil {
		return State{}, err
	}

	for metaName, content := range directorMetas {
		version, err := metaVersion(content)
		if err == nil {
			s.DirectorState[metaName] = MetaState{Version: version, Hash: metaHash(content)}
		}
	}

	targets, err := c.unsafeTargets()
	if err != nil {
		return State{}, err
	}
	for targetName := range targets {
		content, err := c.unsafeTargetFile(targetName)
		if err == nil {
			s.TargetFilenames[targetName] = metaHash(content)
		}
	}

	return s, nil
}
