package util

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/pkg/errors"
)

var (
	// matches <string>/<int>/<string>/<string>/<string> for <type>/<org_id>/<product>/<config_id>/<file>
	filePathRegexp       = regexp.MustCompile(`^([^/]+)/(\d+)/([^/]+)/([^/]+)/([^/]+)$`)
	filePathRegexpGroups = 5
)

type Type uint

const (
	TypeUnknown Type = iota
	TypeDatadog
)

// PathMeta contains the metadata of a specific file contained in its path
type PathMeta struct {
	Type     Type
	OrgID    int64
	Product  pbgo.Product
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
	product, productExists := pbgo.Product_value[rawProduct]
	if !productExists {
		return PathMeta{}, fmt.Errorf("product %s is unknwon", rawProduct)
	}
	return PathMeta{
		Type:     configType,
		OrgID:    orgID,
		Product:  pbgo.Product(product),
		ConfigID: matchedGroups[4],
		Name:     matchedGroups[5],
	}, nil
}
