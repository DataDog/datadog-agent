package gce

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

// declare these as vars not const to ease testing
var (
	metadataURL = "http://169.254.169.254/computeMetadata/v1"
	timeout     = 300 * time.Millisecond
)

// GetHostname returns the hostname querying GCE Metadata api
func GetHostname() (string, error) {
	res, err := getResponse(metadataURL + "/instance/hostname")
	if err != nil {
		return "", fmt.Errorf("unable to retrieve hostname from GCE: %s", err)
	}

	defer res.Body.Close()
	all, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("GCE hostname, error reading response body: %s", err)
	}

	return string(all), nil
}

func getResponse(url string) (*http.Response, error) {
	client := http.Client{
		Timeout: timeout,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Metadata-Flavor", "Google")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status code %d trying to GET %s", res.StatusCode, url)
	}

	return res, nil
}

// HostnameProvider GCE implementation of the HostnameProvider
func HostnameProvider(hostName string) (string, error) {
	return GetHostname()
}
