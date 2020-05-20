package probe

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestMkdirJSON(t *testing.T) {
	e := NewEvent(nil)
	e.Process = ProcessEvent{
		Pidns:   333,
		Comm:    [16]byte{'a', 'a', 'a'},
		TTYName: [64]byte{'b', 'b', 'b'},
		Pid:     123,
		Tid:     456,
		UID:     8,
		GID:     9,
	}
	e.Mkdir = MkdirEvent{
		Inode:       33,
		Mode:        0777,
		MountID:     555,
		PathnameStr: "/etc/passwd",
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}

	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()

	var i interface{}
	err = d.Decode(&i)
	if err != nil {
		t.Fatal(err)
	}
}
