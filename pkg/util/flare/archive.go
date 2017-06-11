package flare

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/jhoonb/archivex"
	yaml "gopkg.in/yaml.v2"
)

// CreateArchive packages up the files
func CreateArchive() (string, error) {
	zipFilePath := mkFilePath()
	return createArchive(zipFilePath)
}

func createArchive(zipFilePath string) (string, error) {
	zipFile := new(archivex.ZipFile)
	zipFile.Create(zipFilePath)
	defer zipFile.Close()

	err := zipLogFiles(zipFile)
	if err != nil {
		fmt.Println("log file error", err)
		return "", err
	}

	err = zipConfigFiles(zipFile)
	if err != nil {
		fmt.Println("config file error", err)
		return "", err
	}

	return zipFilePath, nil
}

func zipLogFiles(zipFile *archivex.ZipFile) error {
	logFile := config.Datadog.GetString("log_file")
	logFilePath := path.Dir(logFile)

	err := filepath.Walk(logFilePath, func(path string, f os.FileInfo, err error) error {
		if f.IsDir() {
			return nil
		}

		if filepath.Ext(f.Name()) == ".log" || getFirstSuffix(f.Name()) == ".log" {
			fileName := filepath.Join("logs", f.Name())
			return zipFile.AddFileWithName(fileName, path)
		}
		return nil
	})

	return err
}

func zipConfigFiles(zipFile *archivex.ZipFile) error {
	c, err := yaml.Marshal(config.Datadog)
	if err != nil {
		return err
	}
	// zip up the actual config
	cleaned, err := credentialsCleanerBytes(c)
	if err != nil {
		return err
	}
	zipFile.Add("datadog.yaml", cleaned)

	err = filepath.Walk(config.Datadog.GetString("confd_path"), func(path string, f os.FileInfo, err error) error {
		if f.IsDir() {
			return nil
		}

		if filepath.Ext(f.Name()) == ".example" {
			return nil
		}

		if getFirstSuffix(f.Name()) == ".yaml" || filepath.Ext(f.Name()) == ".yaml" {
			baseName := strings.Replace(path, config.Datadog.GetString("confd_path"), "", 1)
			fileName := filepath.Join("etc/confd", baseName)
			file, err := credentialsCleanerFile(path)
			if err != nil {
				return err
			}
			return zipFile.Add(fileName, file)
		}

		return nil
	})
	if err != nil {
		return err
	}

	if config.Datadog.ConfigFileUsed() != "" {
		// zip up the config file that was actually used, if one exists
		file, e := credentialsCleanerFile(config.Datadog.ConfigFileUsed())
		if err != nil {
			return e
		}
		e = zipFile.Add("etc/datadog.yaml", file)
		if e != nil {
			return e
		}
	}

	return err
}

func getFirstSuffix(s string) string {
	return filepath.Ext(strings.TrimSuffix(s, filepath.Ext(s)))
}

func mkFilePath() string {
	dir := os.TempDir()
	t := time.Now()
	timeString := t.Format("2006-01-02-15-04-05")
	fileName := strings.Join([]string{"datadog", "agent", timeString}, "-")
	fileName = strings.Join([]string{fileName, "zip"}, ".")
	filePath := path.Join(dir, fileName)
	return filePath
}
