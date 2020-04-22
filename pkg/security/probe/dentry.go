package probe

import (
	"C"

	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/iovisor/gobpf/bcc"
)

// handleDentryEvent - Handles a dentry event
func (p *Probe) handleDentryEvent(data []byte) {
	eventRaw := &model.DentryEventRaw{}
	err := eventRaw.UnmarshalBinary(data)
	if err != nil {
		log.Println("failed to decode received data")
		return
	}

	event := model.DentryEvent{
		EventBase: model.EventBase{
			EventType: eventRaw.GetProbeEventType(),
			Timestamp: p.StartTime.Add(time.Duration(eventRaw.TimestampRaw) * time.Nanosecond),
		},
		DentryEventRaw: eventRaw,
		TTYName:        C.GoString((*C.char)(unsafe.Pointer(&eventRaw.TTYNameRaw))),
	}

	event.SrcFilename, err = p.resolveDentryPath(eventRaw.SrcPathnameKey)
	if err != nil {
		log.Printf("failed to resolve dentry path: %s\n", err)
	}

	switch event.EventType {
	case model.FileHardLinkEventType, model.FileRenameEventType:
		event.TargetFilename, err = p.resolveDentryPath(eventRaw.TargetPathnameKey)
		if err != nil {
			log.Printf("failed to resolve dentry path: %s", err)
		}
	}

	p.DispatchEvent(&event)
}

// resolveDentryPath - Resolve the pathname of a dentry, starting at the pathnameKey in the pathnames table
func (p *Probe) resolveDentryPath(pathnameKey uint32) (string, error) {
	// Don't resolve path if pathnameKey isn't valid
	if pathnameKey <= 0 {
		return "", fmt.Errorf("invalid pathname key %v", pathnameKey)
	}
	table := p.Tables["pathnames"]
	if table == nil {
		return "", fmt.Errorf("pathnames BPF_HASH table doesn't exist")
	}
	// Convert key into bytes
	key := make([]byte, 4)
	binary.LittleEndian.PutUint32(key, pathnameKey)
	filename := ""
	done := false
	pathRaw := []byte{}
	var path struct {
		ParentKey uint32
		Name      [255]byte
	}
	var err1, err2 error
	// Fetch path recursively
	for !done {
		pathRaw, err1 = table.Get(key)
		if err1 != nil {
			filename = "*ERROR*" + filename
			break
		}
		err1 = binary.Read(bytes.NewBuffer(pathRaw), bcc.GetHostByteOrder(), &path)
		if err1 != nil {
			err1 = fmt.Errorf("failed to decode received data (pathLeaf): %s", err1)
			done = true
		}
		// Delete key
		if err2 = table.Delete(key); err2 != nil {
			err1 = fmt.Errorf("pathnames map deletion error: %v", err2)
		}
		if done {
			break
		}
		// Don't append dentry name if this is the root dentry (i.d. name == '/')
		if path.Name[0] != '/' {
			filename = "/" + C.GoString((*C.char)(unsafe.Pointer(&path.Name))) + filename
		}
		if path.ParentKey == 0 {
			break
		}
		// Prepare next key
		binary.LittleEndian.PutUint32(key, path.ParentKey)
	}
	if len(filename) == 0 {
		filename = "/"
	}

	return filename, err1
}
