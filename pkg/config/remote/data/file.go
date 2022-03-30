package data

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/pkg/errors"
)

var (
	// matches <string>/<int>/<string>/<string>/<string> for <type>/<org_id>/<product>/<config_id>/<file>
	filePathRegexp       = regexp.MustCompile(`^([^/]+)/(\d+)/([^/]+)/([^/]+)/([^/]+)$`)
	filePathRegexpGroups = 5
)

// Type is the global type the file belongs to
type Type uint

const (
	// TypeUnknown is an unknown type
	TypeUnknown Type = iota
	// TypeDatadog is the datadog type
	TypeDatadog
)

// PathMeta contains the metadata of a specific file contained in its path
type PathMeta struct {
	Type     Type
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
	rawType := matchedGroups[1]
	configType := TypeUnknown
	switch rawType {
	case "datadog":
		configType = TypeDatadog
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
		Type:     configType,
		OrgID:    orgID,
		Product:  Product(rawProduct),
		ConfigID: matchedGroups[4],
		Name:     matchedGroups[5],
	}, nil
}
