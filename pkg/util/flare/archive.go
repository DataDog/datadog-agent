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

func CreateArchive() (string, error) {
	return createArchive()
}

func createArchive() (string, error) {
	zipFilePath := mkFilePath()
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

	fmt.Println("logFilePath", logFilePath)

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
	fmt.Println("config", config.Datadog)
	c, err := yaml.Marshal(config.Datadog)
	if err != nil {
		return err
	}
	// zip up the actual config
	zipFile.Add("datadog.yaml", c)

	err = filepath.Walk(config.Datadog.GetString("confd_path"), func(path string, f os.FileInfo, err error) error {
		if f.IsDir() {
			return nil
		}

		if filepath.Ext(f.Name()) == ".example" {
			return nil
		}

		if getFirstSuffix(f.Name()) == ".yaml" || filepath.Ext(f.Name()) == ".yaml" {
			baseDir := strings.Replace(path, config.Datadog.GetString("confd_path"), "", 1)

			fileName := filepath.Join("etc/confd", baseDir)
			return zipFile.AddFileWithName(fileName, path)
		}

		return nil
	})
	fmt.Println("config file used: ", config.Datadog.ConfigFileUsed())
	if config.Datadog.ConfigFileUsed() != "" {
		// zip up the config file that was actually used, if one exists
		err = zipFile.AddFileWithName("etc/datadog.yaml", config.Datadog.ConfigFileUsed())
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
