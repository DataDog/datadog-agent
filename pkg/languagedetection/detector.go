package languagedetection

import (
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
)

type Detector interface {
	DetectLanguage(pid int) (languagemodels.Language, error)
}
