package topology

import "fmt"

// Instance is a representation of a topology source instance
type Instance struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// GoString prints as string
func (i *Instance) GoString() string {
	return fmt.Sprintf("type_%s_url_%s", i.Type, i.URL)
}
