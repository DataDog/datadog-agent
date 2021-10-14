package meta

import _ "embed"

// RootConfig is the root of the config repo
//go:embed config.json
var RootConfig []byte

// RootDirector is the root of the director repo
//go:embed director.json
var RootDirector []byte
