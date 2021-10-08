// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package legacy

import (
	"github.com/go-ini/ini"
)

var (
	// whitelist the sections we want to import
	processAgentSections = map[string]struct{}{
		"DEFAULT":        {}, // removing this section would mess up the ini file
		"process.config": {},
	}
)

// ImportProcessAgentConfig reads `datadog.conf` and returns an ini config object,
// ready to be dumped to a .ini file.
func ImportProcessAgentConfig(datadogConfPath, processAgentConfPath string) (bool, error) {
	// read datadog.conf
	iniFile, err := ini.Load(datadogConfPath)
	if err != nil {
		return false, err
	}

	// remove any section that's not process-agent specific
	for _, section := range iniFile.SectionStrings() {
		if _, found := processAgentSections[section]; !found {
			iniFile.DeleteSection(section)
		}
	}

	// only write the file if we have other Sections than DEFAULT
	if len(iniFile.SectionStrings()) > 1 {
		return true, iniFile.SaveTo(processAgentConfPath)
	}

	return false, nil
}
