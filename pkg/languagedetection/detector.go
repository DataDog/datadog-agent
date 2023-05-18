package languagedetection

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

type Language struct {
	Name string
}

// Maps the prefix to the language name
var knownPrefixes = map[string]string{
	"python": "python",
	"java":   "java",
}

func languageFromCommandline(cmdline []string) (string, error) {
	exe := getExe(cmdline)
	for prefix, languageName := range knownPrefixes {
		if strings.HasPrefix(exe, prefix) {
			return languageName, nil
		}
	}
	return "", fmt.Errorf("unknown executable: %s", exe)
}

func DetectLanguage(logger log.Component, procs []procutil.Process) []*Language {
	langs := make([]*Language, len(procs))
	for i, proc := range procs {
		languageName, err := languageFromCommandline(proc.Cmdline)
		if err != nil {
			logger.Trace(languageName)
		}
		langs[i] = &Language{}
	}
	return langs
}
