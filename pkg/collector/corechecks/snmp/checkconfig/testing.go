package checkconfig

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/config"
	"path/filepath"
)

func setConfdPathAndCleanProfiles() {
	GlobalProfileConfigMap = nil // make sure from the new confd path will be reloaded
	file, _ := filepath.Abs(filepath.Join("..", "test", "conf.d"))
	fmt.Println("file: " + file)
	config.Datadog.Set("confd_path", file)
}
