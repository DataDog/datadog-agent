package util

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/jhoonb/archivex"
)

var datadogSupportURL = "/support/flare"

// SendFlare will send a flare
func SendFlare(caseID string, email string) error {

	return nil
}

func sendFlare(url string, caseID string, email string) error {
	archivePath, err := createArchive()
	if err != nil {
		return err
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	p, err := writer.CreateFormFile("flare_file", archivePath)
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	_, err = io.Copy(p, file)
	if err != nil {
		return err
	}
	writer.WriteField("case_id", caseID)
	writer.WriteField("hostname", GetHostname())
	writer.WriteField("email", email)

	err = writer.Close()

	if err != nil {
		return err
	}

	request, err := http.NewRequest("POST", url, body)

	client := &http.Client{}

	_, err = client.Do(request)
	if err != nil {
		return err
	}

	return nil
}

func createArchive() (string, error) {
	zipFilePath := mkFilePath()
	zipFile := new(archivex.ZipFile)
	zipFile.Create(zipFilePath)
	defer zipFile.Close()

	logFile := config.Datadog.GetString("log_file")
	logFilePath := path.Dir(logFile)

	c, err := yaml.Marshal(&config.Datadog)
	if err != nil {
		return "", err
	}
	// zip up the actual config
	zipFile.Add("datadog.yaml", c)

	filepath.Walk(logFilePath, func(path string, f os.FileInfo, err error) error {
		if f.IsDir() {
			return nil
		}
		fileName := filepath.Join("logs", f.Name())
		return zipFile.AddFileWithName(path, fileName)
	})

	filepath.Walk(config.Datadog.GetString("confd_path"), func(path string, f os.FileInfo, err error) error {
		if f.IsDir() {
			return nil
		}

		baseDir := strings.Replace(path, config.Datadog.GetString("confd_path"), "", 1)

		fileName := filepath.Join("etc", baseDir, f.Name())
		return zipFile.AddFileWithName(path, fileName)
	})
	// zip up the config file that was actually used
	zipFile.AddFileWithName(config.Datadog.ConfigFileUsed(), "etc/datadog.yaml")

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
