// TODO: Need refactoring, copied from snmp corecheck integration

package fetch

import (
	"strconv"
)

type columnFetchStrategy int

const (
	UseGetBulk columnFetchStrategy = iota
	UseGetNext
)

func (c columnFetchStrategy) String() string {
	switch c {
	case UseGetBulk:
		return "UseGetBulk"
	case UseGetNext:
		return "UseGetNext"
	default:
		return strconv.Itoa(int(c))
	}
}
