// +build linux windows darwin
// I don't think windows and darwin can actually be docker hosts
// but keeping it this way for build consistency (for now)

package util

import (
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	log "github.com/cihub/seelog"
)

func getContainerHostname() (bool, string) {
	var name string
	if isContainerized() {
		// Docker
		log.Debug("GetHostname trying Docker API...")
		if getDockerHostname, found := hostname.ProviderCatalog["docker"]; found {
			name, err := getDockerHostname(name)
			if err == nil && ValidHostname(name) == nil {
				return true, name
			} else if isKubernetes() {
				log.Debug("GetHostname trying k8s...")
				// TODO
			}
		}
	}

	return false, name
}
