package flare

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
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

	// Get hostname from cache if it exists there
	// Otherwise, create it.
	// If it is not in the cache,
	// it's likely because it was unable to run this on the agent itself
	var hostname string
	x, found := util.Cache.Get("hostname")
	if found {
		hostname = x.(string)
	} else {
		hostname = util.GetHostname()
	}

	defer zipFile.Close()

	err := zipLogFiles(zipFile, hostname)
	if err != nil {
		return "", err
	}

	err = zipConfigFiles(zipFile, hostname)
	if err != nil {
		return "", err
	}

	return zipFilePath, nil
}

func zipLogFiles(zipFile *archivex.ZipFile, hostname string) error {
	logFile := config.Datadog.GetString("log_file")
	logFilePath := path.Dir(logFile)

	err := filepath.Walk(logFilePath, func(path string, f os.FileInfo, err error) error {
		if f.IsDir() {
			return nil
		}

		if filepath.Ext(f.Name()) == ".log" || getFirstSuffix(f.Name()) == ".log" {
			fileName := filepath.Join(hostname, "logs", f.Name())
			return zipFile.AddFileWithName(fileName, path)
		}
		return nil
	})

	return err
}

func zipConfigFiles(zipFile *archivex.ZipFile, hostname string) error {
	c, err := yaml.Marshal(config.Datadog.AllSettings())
	if err != nil {
		return err
	}
	// zip up the actual config
	cleaned, err := credentialsCleanerBytes(c)
	if err != nil {
		return err
	}
	err = zipFile.Add(filepath.Join(hostname, "datadog.yaml"), cleaned)
	if err != nil {
		return err
	}

	err = filepath.Walk(config.Datadog.GetString("confd_path"), func(path string, f os.FileInfo, err error) error {
		if f.IsDir() {
			return nil
		}

		if filepath.Ext(f.Name()) == ".example" {
			return nil
		}

		if getFirstSuffix(f.Name()) == ".yaml" || filepath.Ext(f.Name()) == ".yaml" {
			baseName := strings.Replace(path, config.Datadog.GetString("confd_path"), "", 1)
			fileName := filepath.Join(hostname, "etc", "confd", baseName)
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
		fileName := filepath.Join(hostname, "etc", "datadog.yaml")
		e = zipFile.Add(fileName, file)
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
