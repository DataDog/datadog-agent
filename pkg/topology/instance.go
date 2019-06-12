package topology

import "fmt"

type Instance struct {
	Type string `json:"type"`
	Url string `json:"url"`
}

func (i *Instance) GoString() string {
	return fmt.Sprintf("type_%s_url_%s", i.Type, i.Url)
}
