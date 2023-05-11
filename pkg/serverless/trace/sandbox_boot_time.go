package trace

import (
	"os"
	"time"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func GetSandboxBootTime() (*time.Time, error) {
	filePath := "/proc/1/cmdline"
	file, err := os.Open(filePath)
	defer file.Close()

	if err != nil {
		log.Debugf("Error opening file: %s\n", err)
		return nil, err
	}

	// Getting file information
	fileInfo, err := file.Stat()
	if err != nil {
		log.Debugf("Error getting file info: %s\n", err)
		return nil, err
	}

	// Converting the modification time to Unix nanoseconds
	modTime := fileInfo.ModTime()

	return &modTime, nil
}
