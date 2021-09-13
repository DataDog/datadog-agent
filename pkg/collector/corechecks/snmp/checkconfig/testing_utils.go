package checkconfig

import (
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// SetConfdPathAndCleanProfiles is used for testing only
func SetConfdPathAndCleanProfiles() {
	globalProfileConfigMap = nil // make sure from the new confd path will be reloaded
	file, _ := filepath.Abs(filepath.Join(".", "test", "conf.d"))
	if !pathExists(file) {
		file, _ = filepath.Abs(filepath.Join("..", "test", "conf.d"))
	}
	config.Datadog.Set("confd_path", file)
}

// pathExists returns true if the given path exists
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
