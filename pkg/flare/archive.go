package flare

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/jhoonb/archivex"
	yaml "gopkg.in/yaml.v2"
)

// CreateArchive packages up the files
func CreateArchive(local bool) (string, error) {
	zipFilePath := mkFilePath()
	return createArchive(zipFilePath, local)
}

func createArchive(zipFilePath string, local bool) (string, error) {
	zipFile := new(archivex.ZipFile)
	zipFile.Create(zipFilePath)

	// Get hostname, if there's an error in getting the hostname,
	// set the hostname to unknown
	hostname, err := util.GetHostname()
	if err != nil {
		hostname = "unknown"
	}

	defer zipFile.Close()

	if local {
		zipFile.Add(filepath.Join(hostname, "local"), []byte{})
	}

	err = zipLogFiles(zipFile, hostname)
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
		if f == nil {
			return nil
		}
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

	err = walkConfigFilePaths(zipFile, hostname)
	if err != nil {
		return err
	}

	if config.Datadog.ConfigFileUsed() != "" {
		// zip up the config file that was actually used, if one exists
		filePath := config.Datadog.ConfigFileUsed()
		// Check if the file exists
		_, e := os.Stat(filePath)
		if e != nil {
			file, e := credentialsCleanerFile(filePath)
			if err != nil {
				return e
			}
			fileName := filepath.Join(hostname, "etc", "datadog.yaml")
			e = zipFile.Add(fileName, file)
			if e != nil {
				return e
			}
		}
	}

	return err
}

func walkConfigFilePaths(zipFile *archivex.ZipFile, hostname string) error {
	confSearchPaths := map[string]string{
		"":        config.Datadog.GetString("confd_path"),
		"dist":    filepath.Join(common.GetDistPath(), "conf.d"),
		"checksd": common.PyChecksPath,
	}
	for prefix, filePath := range confSearchPaths {
		err := filepath.Walk(filePath, func(path string, f os.FileInfo, err error) error {
			if f == nil {
				return nil
			}
			if f.IsDir() {
				return nil
			}

			if filepath.Ext(f.Name()) == ".example" {
				return nil
			}

			if getFirstSuffix(f.Name()) == ".yaml" || filepath.Ext(f.Name()) == ".yaml" {
				baseName := strings.Replace(path, filePath, "", 1)
				fileName := filepath.Join(hostname, "etc", "confd", prefix, baseName)
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

	}

	return nil
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
