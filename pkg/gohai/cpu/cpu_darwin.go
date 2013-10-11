package cpu

import (
	"debug/macho"
)

type Cpu struct{}

func (self *Cpu) Collect() (result map[string]string, err error) {
	cpu := (&macho.FileHeader{}).Cpu

	return map[string]string{
		"cpu": cpu.String(),
	}, err
}
