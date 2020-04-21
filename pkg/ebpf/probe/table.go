package probe

import (
	"fmt"
	"log"

	"github.com/iovisor/gobpf/bcc"
)

type Table struct {
	*bcc.Table

	Name string
}

func (t *Table) Register(module *Module) error {
	if t.Table = bcc.NewTable(module.TableId(t.Name), module.Module); t.Table == nil {
		return fmt.Errorf("could not register table %s", t.Name)
	}

	log.Printf("Table %s registered", t.Name)
	return nil
}
