package util

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/jhoonb/archivex"
)

var datadogSupportURL = "/support/flare"

// SendFlareWithCaseID will send a flare with a caseID
func SendFlareWithCaseID(caseID string) error {

	return nil
}

// SendFlare will send a flare
func SendFlare() error {

	return nil
}

func createArchive() (string, error) {
	zipFilePath := mkFilePath()
	zipFile := new(archivex.ZipFile)
	zipFile.Create(zipFilePath)
	defer zipFile.Close()

	logFile := config.Datadog.GetString("log_file")
	logFilePath := path.Dir(logFile)

	filepath.Walk(logFilePath, func(path string, f os.FileInfo, err error) error {
		if f.IsDir() {
			return nil
		}

		fileName := filepath.Join("logs", f.Name())

		zipFile.AddFileWithName(path, fileName)

		return nil
	})

	return zipFilePath, nil
}

func mkFilePath() string {
	dir := os.TempDir()
	t := time.Now()
	timeString := t.Format("2006-01-02-15-04-05")
	fileName := strings.Join([]string{dir, "datadog-agent-", timeString}, "-")
	fileName = strings.Join([]string{fileName, "zip"}, ".")
	return fileName
}
