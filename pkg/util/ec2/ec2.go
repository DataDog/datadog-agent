package ec2

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

const (
	metadataURL               = "http://169.254.169.254/latest/meta-data/"
	timeout     time.Duration = 100 * time.Millisecond
)

var (
	defaultPrefixes = []string{"ip-", "domu"}
)

// GetInstanceID returns the EC2 instance id for this host
func GetInstanceID() (string, error) {
	return getInstanceID(metadataURL + "instance-id")
}

func getInstanceID(url string) (string, error) {
	client := http.Client{
		Timeout: timeout,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("status code %d trying to fetch %s", res.StatusCode, url)
	}

	responseData, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body, %s", err)
	}

	return string(responseData), nil
}

// IsDefaultHostname returns whether the given hostname is a default one for EC2
func IsDefaultHostname(hostname string) bool {
	hostname = strings.ToLower(hostname)
	isDefault := false
	for _, val := range defaultPrefixes {
		isDefault = isDefault || strings.HasPrefix(hostname, val)
	}
	return isDefault
}
