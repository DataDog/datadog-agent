package profile

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"os"
	"path/filepath"
)

// SetConfdPathAndCleanProfiles is used for testing only
func SetConfdPathAndCleanProfiles() {
	SetGlobalProfileConfigMap(nil) // make sure from the new confd path will be reloaded
	file, _ := filepath.Abs(filepath.Join(".", "test", "conf.d"))
	if !pathExists(file) {
		file, _ = filepath.Abs(filepath.Join("..", "test", "conf.d"))
	}
	if !pathExists(file) {
		file, _ = filepath.Abs(filepath.Join(".", "internal", "test", "conf.d"))
	}
	config.Datadog.Set("confd_path", file)
}

// pathExists returns true if the given path exists
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
