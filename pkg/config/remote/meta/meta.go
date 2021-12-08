package meta

import (
	_ "embed"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var (
	//go:embed 1.director.json
	rootDirector1 []byte

	//go:embed 1.config.json
	rootConfig1 []byte
)

type EmbeddedRoot []byte
type EmbeddedRoots map[uint64]EmbeddedRoot

var rootsDirector = EmbeddedRoots{
	1: rootDirector1,
}

var rootsConfig = EmbeddedRoots{
	1: rootConfig1,
}

func RootsDirector() EmbeddedRoots {
	if directorRoot := config.Datadog.GetString("remote_configuration.director_root"); directorRoot != "" {
		return EmbeddedRoots{
			1: EmbeddedRoot(directorRoot),
		}
	}
	return rootsDirector
}

func RootsConfig() EmbeddedRoots {
	if configRoot := config.Datadog.GetString("remote_configuration.config_root"); configRoot != "" {
		return EmbeddedRoots{
			1: EmbeddedRoot(configRoot),
		}
	}
	return rootsConfig
}

func (roots EmbeddedRoots) Last() EmbeddedRoot {
	return roots[roots.LastVersion()]
}

func (roots EmbeddedRoots) LastVersion() uint64 {
	return uint64(len(roots))
}
