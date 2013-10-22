package hostname

import (
	"os"
)

type Hostname struct{}

const name = "hostname"

func (self *Hostname) Name() string {
	return name
}

func (self *Hostname) Collect() (result interface{}, err error) {
	hostname, err := os.Hostname()
	result = hostname

	return
}
