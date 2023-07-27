package languagedetection

import (
	"os"
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/internal/detectors"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var detectorsWithPrivilege = []Detector{
	detectors.GoDetector{},
}

var (
	PermissionDeniedWarningOnce = sync.Once{}
)

func handleDetectorError(err error) {
	if os.IsPermission(err) {
		PermissionDeniedWarningOnce.Do(func() {
			log.Warnf("Attempted to detect language but permission was denied. Make sure the system probe is running as root and has CAP_PTRACE if it is running in a container.")
		})
	}
}

func DetectWithPrivileges(pids []int) []languagemodels.Language {
	languages := make([]languagemodels.Language, len(pids))
	for i, pid := range pids {
		for _, detector := range detectorsWithPrivilege {
			lang, err := detector.DetectLanguage(pid)
			if err != nil {
				handleDetectorError(err)
				continue
			}
			languages[i] = lang
		}
	}
	return languages
}

func MockPrivilegedDetectors(t *testing.T, newDetectors []Detector) {
	oldDetectors := detectorsWithPrivilege
	t.Cleanup(func() { detectorsWithPrivilege = oldDetectors })
	detectorsWithPrivilege = newDetectors
}
