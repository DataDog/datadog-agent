package catalog

import "fmt"

type Catalog struct {
	Versions map[string]*Version
	Latest   string
}

type Version struct {
	Hash   string
	Source string // URL where to fetch it from

	// must support multiple archs/os too
}

func (c *Catalog) GetVersion(version string) (*Version, error) {
	v, ok := c.Versions[version]
	if !ok {
		return nil, fmt.Errorf("Unknown version: %s", version)
	}
	return v, nil
}

func (c *Catalog) GetLatest() string {
	return c.Latest
}

func GetMockedCatalog() Catalog {
	versions := map[string]*Version{
		"7.48.0-1": {
			Hash:   "08ce838dd6d6b61fbf5c821cb234eb07fe85b1d02f3e6ce39d4a436cdf0efacc",
			Source: "7.48.0-1", // version to use for apt-get
		},
		"7.47.0-1": {
			Hash:   "1bf15b4ee538bacec9c49c858f72df96def4095d5639163ea31d142449023127",
			Source: "7.47.0-1",
		},
	}
	return Catalog{
		Versions: versions,
		Latest:   "7.48.0-1",
	}
}
