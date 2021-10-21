package discovery

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// SetTestRunPath sets run_path for testing
func SetTestRunPath() {
	path, _ := filepath.Abs(filepath.Join(".", "test", "run_path"))
	config.Datadog.Set("run_path", path)
}
