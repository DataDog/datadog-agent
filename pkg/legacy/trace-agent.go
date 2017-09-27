// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package legacy

import (
	"fmt"

	"github.com/go-ini/ini"
)

var (
	traceAgentSections = map[string]struct{}{
		"DEFAULT":        struct{}{}, // removing this section would mess up the ini file
		"trace.sampler":  struct{}{},
		"trace.receiver": struct{}{},
		"trace.ignore":   struct{}{},
	}
)

// ImportTraceAgentConfig reads `datadog.conf` and returns an ini config object,
// ready to be dumped to a .ini file.
func ImportTraceAgentConfig(datadogConfPath, traceAgentConfPath string) error {
	// read datadog.conf
	iniFile, err := ini.Load(datadogConfPath)
	if err != nil {
		return err
	}

	// remove any section that's not trace-agent specific
	for _, section := range iniFile.SectionStrings() {
		if _, found := traceAgentSections[section]; !found {
			iniFile.DeleteSection(section)
		}
	}

	// only dump the file if we have Sections
	if len(iniFile.SectionStrings()) > 0 {
		return iniFile.SaveTo(traceAgentConfPath)
	}

	return fmt.Errorf("nothing to import")
}
