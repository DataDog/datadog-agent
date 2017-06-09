package flare

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util"
)

var datadogSupportURL = "/support/flare"

// SendFlare will send a flare
func SendFlare(archivePath string, url string, caseID string, email string) error {

	return nil
}

func sendFlare(archivePath string, url string, caseID string, email string) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	p, err := writer.CreateFormFile("flare_file", archivePath)
	if err != nil {
		return err
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	_, err = io.Copy(p, file)
	if err != nil {
		return err
	}
	writer.WriteField("case_id", caseID)
	writer.WriteField("hostname", util.GetHostname())
	writer.WriteField("email", email)

	err = writer.Close()

	if err != nil {
		return err
	}

	request, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err
	}

	client := &http.Client{}

	_, err = client.Do(request)
	if err != nil {
		return err
	}

	return nil
}
