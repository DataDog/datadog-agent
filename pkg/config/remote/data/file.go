package data

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/pkg/errors"
)

var (
	// matches <string>/<int>/<string>/<string>/<string> for <source>/<org_id>/<product>/<config_id>/<file>
	filePathRegexp       = regexp.MustCompile(`^([^/]+)/(\d+)/([^/]+)/([^/]+)/([^/]+)$`)
	filePathRegexpGroups = 5
)

// Source is the source of the config file
type Source uint

const (
	// SourceUnknown is an unknown source
	SourceUnknown Source = iota
	// SourceDatadog is the datadog source
	SourceDatadog
	// SourceUser is the user source
	SourceUser
)

// PathMeta contains the metadata of a specific file contained in its path
type PathMeta struct {
	Source   Source
	OrgID    int64
	Product  Product
	ConfigID string
	Name     string
}

// ParseFilePathMeta parses a file path meta
func ParseFilePathMeta(path string) (PathMeta, error) {
	matchedGroups := filePathRegexp.FindStringSubmatch(path)
	if len(matchedGroups) != filePathRegexpGroups+1 {
		return PathMeta{}, fmt.Errorf("config file path '%s' has wrong format", path)
	}
	rawSource := matchedGroups[1]
	configSource := SourceUnknown
	switch rawSource {
	case "datadog":
		configSource = SourceDatadog
	case "user":
		configSource = SourceUser
	}
	rawOrgID := matchedGroups[2]
	orgID, err := strconv.ParseInt(rawOrgID, 10, 64)
	if err != nil {
		return PathMeta{}, errors.Wrapf(err, "could not parse orgID '%s' in config file path", rawOrgID)
	}
	rawProduct := matchedGroups[3]
	if len(rawProduct) == 0 {
		return PathMeta{}, fmt.Errorf("product is empty")
	}
	return PathMeta{
		Source:   configSource,
		OrgID:    orgID,
		Product:  Product(rawProduct),
		ConfigID: matchedGroups[4],
		Name:     matchedGroups[5],
	}, nil
}
