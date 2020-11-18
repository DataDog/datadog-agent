package config

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/gopsutil/process"
)

var (
	defaultSensitiveWords = []string{
		"*password*", "*passwd*", "*mysql_pwd*",
		"*access_token*", "*auth_token*",
		"*api_key*", "*apikey*",
		"*secret*", "*credentials*", "stripetoken"}
)

const (
	defaultCacheMaxCycles = 25
)

// DataScrubber allows the agent to blacklist cmdline arguments that match
// a list of predefined and custom words
type DataScrubber struct {
	Enabled           bool
	StripAllArguments bool
	SensitivePatterns []*regexp.Regexp
	seenProcess       map[string]struct{}
	scrubbedCmdlines  map[string][]string
	cacheCycles       uint32 // used to control the cache age
	cacheMaxCycles    uint32 // number of cycles before resetting the cache content
}

// NewDefaultDataScrubber creates a DataScrubber with the default behavior: enabled
// and matching the default sensitive words
func NewDefaultDataScrubber() *DataScrubber {
	newDataScrubber := &DataScrubber{
		Enabled:           true,
		SensitivePatterns: CompileStringsToRegex(defaultSensitiveWords),
		seenProcess:       make(map[string]struct{}),
		scrubbedCmdlines:  make(map[string][]string),
		cacheCycles:       0,
		cacheMaxCycles:    defaultCacheMaxCycles,
	}

	return newDataScrubber
}

// CompileStringsToRegex compile each word in the slice into a regex pattern to match
// against the cmdline arguments
// The word must contain only word characters ([a-zA-z0-9_]) or wildcards *
func CompileStringsToRegex(words []string) []*regexp.Regexp {
	compiledRegexps := make([]*regexp.Regexp, 0, len(words))
	forbiddenSymbols := regexp.MustCompile("[^a-zA-Z0-9_*]")

	for _, word := range words {
		if forbiddenSymbols.MatchString(word) {
			log.Warnf("data scrubber: %s skipped. The sensitive word must "+
				"contain only alphanumeric characters, underscores or wildcards ('*')", word)
			continue
		}

		if word == "*" {
			log.Warn("data scrubber: ignoring wildcard-only ('*') sensitive word as it is not supported")
			continue
		}

		originalRunes := []rune(word)
		var enhancedWord bytes.Buffer
		valid := true
		for i, rune := range originalRunes {
			if rune == '*' {
				if i == len(originalRunes)-1 {
					enhancedWord.WriteString("[^ =:]*")
				} else if originalRunes[i+1] == '*' {
					log.Warnf("data scrubber: %s skipped. The sensitive word "+
						"must not contain two consecutives '*'", word)
					valid = false
					break
				} else {
					enhancedWord.WriteString(fmt.Sprintf("[^\\s=:$/]*"))
				}
			} else {
				enhancedWord.WriteString(string(rune))
			}
		}

		if !valid {
			continue
		}

		pattern := "(?P<key>( +| -{1,2})(?i)" + enhancedWord.String() + ")(?P<delimiter> +|=|:)(?P<value>[^\\s]*)"
		r, err := regexp.Compile(pattern)
		if err == nil {
			compiledRegexps = append(compiledRegexps, r)
		} else {
			log.Warnf("data scrubber: %s skipped. It couldn't be compiled into a regex expression", word)
		}
	}

	return compiledRegexps
}

// createProcessKey returns an unique identifier for a given process
func createProcessKey(p *process.FilledProcess) string {
	var b bytes.Buffer
	b.WriteString("p:")
	b.WriteString(strconv.Itoa(int(p.Pid)))
	b.WriteString("|c:")
	b.WriteString(strconv.Itoa(int(p.CreateTime)))

	return b.String()
}

// ScrubProcessCommand uses a cache memory to avoid scrubbing already known
// process' cmdlines
func (ds *DataScrubber) ScrubProcessCommand(p *process.FilledProcess) []string {
	if ds.StripAllArguments {
		return ds.stripArguments(p.Cmdline)
	}

	if !ds.Enabled {
		return p.Cmdline
	}

	pKey := createProcessKey(p)
	if _, ok := ds.seenProcess[pKey]; !ok {
		ds.seenProcess[pKey] = struct{}{}
		if scrubbed, changed := ds.ScrubCommand(p.Cmdline); changed {
			ds.scrubbedCmdlines[pKey] = scrubbed
		}
	}

	if scrubbed, ok := ds.scrubbedCmdlines[pKey]; ok {
		return scrubbed
	}
	return p.Cmdline
}

// IncrementCacheAge increments one cycle of cache memory age. If it reaches
// cacheMaxCycles, the cache is restarted
func (ds *DataScrubber) IncrementCacheAge() {
	ds.cacheCycles++
	if ds.cacheCycles == ds.cacheMaxCycles {
		ds.seenProcess = make(map[string]struct{})
		ds.scrubbedCmdlines = make(map[string][]string)
		ds.cacheCycles = 0
	}
}

// ScrubCommand hides the argument value for any key which matches a "sensitive word" pattern.
// It returns the updated cmdline, as well as a boolean representing whether it was scrubbed
func (ds *DataScrubber) ScrubCommand(cmdline []string) ([]string, bool) {
	newCmdline := cmdline
	rawCmdline := strings.Join(cmdline, " ")
	changed := false
	for _, pattern := range ds.SensitivePatterns {
		if pattern.MatchString(rawCmdline) {
			changed = true
			rawCmdline = pattern.ReplaceAllString(rawCmdline, "${key}${delimiter}********")
		}
	}

	if changed {
		newCmdline = strings.Split(rawCmdline, " ")
	}
	return newCmdline, changed
}

// Strip away all arguments from the command line
func (ds *DataScrubber) stripArguments(cmdline []string) []string {
	// We will sometimes see the entire command line come in via the first element -- splitting guarantees removal
	// of arguments in these cases.
	if len(cmdline) > 0 {
		return []string{strings.Split(cmdline[0], " ")[0]}
	}
	return cmdline
}

// AddCustomSensitiveWords adds custom sensitive words on the DataScrubber object
func (ds *DataScrubber) AddCustomSensitiveWords(words []string) {
	newPatterns := CompileStringsToRegex(words)
	ds.SensitivePatterns = append(ds.SensitivePatterns, newPatterns...)
}
