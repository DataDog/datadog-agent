// +build linux

package net

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/ebpf/oomkill"
	"github.com/DataDog/datadog-agent/pkg/ebpf/tcpqueuelength"
	"github.com/elastic/go-libaudit"
)

const (
	checksURL = "http://unix/check"
)

// GetCheck returns the output of the specified check
func (r *RemoteSysProbeUtil) GetCheck(check string) ([]interface{}, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/%s", checksURL, check), nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("conn request failed: socket %s, url %s, status code: %d", r.path, fmt.Sprintf("%s/%s", checksURL, check), resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if check == "tcp_queue_length" {
		var stats []tcpqueuelength.Stats
		err = json.Unmarshal(body, &stats)
		if err != nil {
			return nil, err
		}
		s := make([]interface{}, len(stats))
		for i, v := range stats {
			s[i] = v
		}
		return s, nil
	} else if check == "oom_kill" {
		var stats []oomkill.Stats
		err = json.Unmarshal(body, &stats)
		if err != nil {
			return nil, err
		}
		s := make([]interface{}, len(stats))
		for i, v := range stats {
			s[i] = v
		}
		return s, nil
	} else if check == "linux_audit" {
		var status libaudit.AuditStatus
		err = json.Unmarshal(body, &status)
		if err != nil {
			return nil, err
		}
		s := make([]interface{}, 1)
		s[0] = status
		return s, nil
	}

	return nil, fmt.Errorf("Invalid check name: %s", check)
}
