package flare

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
)

var datadogSupportURL = "/support/flare"
var httpTimeout = time.Duration(60)

// SendFlare will send a flare
func SendFlare(archivePath string, caseID string, email string) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	p, err := writer.CreateFormFile("flare_file", archivePath)
	if err != nil {
		return err
	}
	file, err := os.Open(archivePath)
	defer file.Close()
	if err != nil {
		return err
	}
	_, err = io.Copy(p, file)
	if err != nil {
		return err
	}
	if caseID != "" {
		writer.WriteField("case_id", caseID)
	}
	if email != "" {
		writer.WriteField("email", email)
	}
	writer.WriteField("hostname", util.GetHostname())

	err = writer.Close()
	if err != nil {
		return err
	}

	var url = mkURL(caseID)
	request, err := http.NewRequest("POST", url, body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if err != nil {
		return err
	}

	client := mkHTTPClient()
	_, err = client.Do(request)
	if err != nil {
		return err
	}

	return nil
}

func mkHTTPClient() *http.Client {
	transport := util.CreateHTTPTransport()

	client := &http.Client{
		Transport: transport,
		Timeout:   httpTimeout * time.Second,
	}

	return client
}

func mkURL(caseID string) string {
	var url string = config.Datadog.GetString("dd_url") + datadogSupportURL
	if caseID != "" {
		url += "/" + caseID
	}
	url += "?api_key=" + config.Datadog.GetString("api_key")
	return url
}
