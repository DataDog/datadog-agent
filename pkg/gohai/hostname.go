package verity

import (
	"os"
)

type Hostname struct{}

func (self *Hostname) Collect() (result map[string]string, err error) {
	hostname, err := os.Hostname()

	return map[string]string{
		"hostname": hostname,
	}, err
}
