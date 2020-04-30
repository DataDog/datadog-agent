package types

type KProbe struct {
	Name       string
	EntryFunc  string
	EntryEvent string
	ExitFunc   string
	ExitEvent  string
}

type Table struct {
	Name string
}

type PerfMap struct {
	Name         string
	BufferLength int
	Handler      func([]byte)
}
