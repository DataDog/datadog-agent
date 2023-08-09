package detectors

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
)

const jrubyClassName = "org.jruby.Main"

// JRubyDetector is a languagedetection.Detector that detects JRuby processes
type JRubyDetector struct{}

func (d JRubyDetector) DetectLanguage(process languagemodels.Process) (languagemodels.Language, error) {
	if process.GetCommand() != "java" {
		return languagemodels.Language{
			Name: languagemodels.Unknown,
		}, nil
	}

	for _, arg := range process.GetCmdline() {
		if strings.TrimSpace(arg) == jrubyClassName {
			return languagemodels.Language{
				Name: languagemodels.Ruby,
			}, nil
		}
	}

	return languagemodels.Language{
		Name: languagemodels.Unknown,
	}, nil
}
