package probe

import (
	"fmt"
)

type KProbe struct {
	Name       string
	EntryFunc  string
	EntryEvent string
	entryFd    int
	ExitFunc   string
	ExitEvent  string
	exitFd     int
}

func (k *KProbe) Register(module *Module) (err error) {
	if k.EntryFunc != "" {
		if k.entryFd, err = module.LoadKprobe(k.EntryFunc); err != nil {
			return fmt.Errorf("failed to load Kprobe %v: %s", k.EntryFunc, err)
		}

		if err = module.AttachKprobe(k.EntryEvent, k.entryFd, -1); err != nil {
			return fmt.Errorf("failed to attach Kprobe %v: %s", k.EntryEvent, err)
		}
	}
	if k.ExitFunc != "" {
		if k.exitFd, err = module.LoadKprobe(k.ExitFunc); err != nil {
			return fmt.Errorf("failed to load Kprobe %v: %s", k.ExitFunc, err)
		}

		if err = module.AttachKretprobe(k.ExitEvent, k.exitFd, -1); err != nil {
			return fmt.Errorf("failed to attach Kretprobe %v: %s", k.ExitEvent, err)
		}
	}

	return nil
}
