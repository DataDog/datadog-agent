// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package state

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	// matches datadog/<int>/<string>/<string>/<string> for datadog/<org_id>/<product>/<config_id>/<file>
	datadogPathRegexp       = regexp.MustCompile(`^datadog/(\d+)/([^/]+)/([^/]+)/([^/]+)$`)
	datadogPathRegexpGroups = 4
	configIDPrefixRegexp    = regexp.MustCompile(`^(\d+)\.`)

	// matches employee/<string>/<string>/<string> for employee/<org_id>/<product>/<config_id>/<file>
	employeePathRegexp       = regexp.MustCompile(`^employee/([^/]+)/([^/]+)/([^/]+)$`)
	employeePathRegexpGroups = 3
)

// DatadogConfigIDFromPath returns the config ID from a Datadog remote-config path, or an empty string if the path is
// invalid.
func DatadogConfigIDFromPath(path string) string {
	configPath, err := parseDatadogConfigPath(path)
	if err != nil {
		return ""
	}
	return configPath.ConfigID
}

// ConfigIDOrder returns the numeric ordering prefix from a config ID in the form N.<name>. IDs without a valid prefix
// have order zero.
func ConfigIDOrder(configID string) int {
	matches := configIDPrefixRegexp.FindStringSubmatch(configID)
	if len(matches) <= 1 {
		return 0
	}

	order, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	return order
}

// DatadogConfigPathLess orders Datadog remote-config paths by their config IDs' numeric prefixes, then by full path
// for deterministic ordering when prefixes are equal or absent.
func DatadogConfigPathLess(left, right string) bool {
	leftOrder := ConfigIDOrder(DatadogConfigIDFromPath(left))
	rightOrder := ConfigIDOrder(DatadogConfigIDFromPath(right))
	if leftOrder != rightOrder {
		return leftOrder < rightOrder
	}
	return left < right
}

type source uint

const (
	sourceUnknown source = iota
	sourceDatadog
	sourceEmployee
)

type configPath struct {
	Source   source
	OrgID    int64
	Product  string
	ConfigID string
	Name     string
}

func parseConfigPath(path string) (configPath, error) {
	configType := parseConfigPathSource(path)
	switch configType {
	case sourceDatadog:
		return parseDatadogConfigPath(path)
	case sourceEmployee:
		return parseEmployeeConfigPath(path)
	}
	return configPath{}, fmt.Errorf("config path '%s' has unknown source", path)
}

func parseDatadogConfigPath(path string) (configPath, error) {
	matchedGroups := datadogPathRegexp.FindStringSubmatch(path)
	if len(matchedGroups) != datadogPathRegexpGroups+1 {
		return configPath{}, fmt.Errorf("config file path '%s' has wrong format", path)
	}
	rawOrgID := matchedGroups[1]
	orgID, err := strconv.ParseInt(rawOrgID, 10, 64)
	if err != nil {
		return configPath{}, fmt.Errorf("could not parse orgID '%s' in config file path: %v", rawOrgID, err)
	}
	rawProduct := matchedGroups[2]
	if len(rawProduct) == 0 {
		return configPath{}, errors.New("product is empty")
	}
	return configPath{
		Source:   sourceDatadog,
		OrgID:    orgID,
		Product:  rawProduct,
		ConfigID: matchedGroups[3],
		Name:     matchedGroups[4],
	}, nil
}

func parseEmployeeConfigPath(path string) (configPath, error) {
	matchedGroups := employeePathRegexp.FindStringSubmatch(path)
	if len(matchedGroups) != employeePathRegexpGroups+1 {
		return configPath{}, fmt.Errorf("config file path '%s' has wrong format", path)
	}
	rawProduct := matchedGroups[1]
	if len(rawProduct) == 0 {
		return configPath{}, errors.New("product is empty")
	}
	return configPath{
		Source:   sourceEmployee,
		Product:  rawProduct,
		ConfigID: matchedGroups[2],
		Name:     matchedGroups[3],
	}, nil
}

func parseConfigPathSource(path string) source {
	switch {
	case strings.HasPrefix(path, "datadog/"):
		return sourceDatadog
	case strings.HasPrefix(path, "employee/"):
		return sourceEmployee
	}
	return sourceUnknown
}
