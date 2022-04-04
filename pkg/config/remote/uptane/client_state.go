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
