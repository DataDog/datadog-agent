package probe

import (
	"C"

	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"unsafe"

	"github.com/davecgh/go-spew/spew"
	"github.com/iovisor/gobpf/bcc"

	eprobe "github.com/DataDog/datadog-agent/pkg/ebpf/probe"
)

// handleDentryEvent - Handles a dentry event
func (p *Probe) handleDentryEvent(data []byte) {
	log.Println("Handling dentry event")

	offset := 0
	event := &Event{}

	read, err := event.Event.UnmarshalBinary(data)
	if err != nil {
		log.Println("failed to decode event")
		return
	}
	offset += read

	read, err = event.Process.UnmarshalBinary(data[offset:])
	if err != nil {
		log.Println("failed to decode process event")
		return
	}
	offset += read

	switch ProbeEventType(event.Event.Type) {
	case FileMkdirEventType:
		if _, err := event.Mkdir.UnmarshalBinary(data[offset:]); err != nil {
			log.Println("failed to decode received data")
			return
		}
	default:
		log.Printf("Unsupported event type %d\n", event.Event.Type)
	}

	log.Printf("Dispatching event %s\n", spew.Sdump(event))
	p.DispatchEvent(event)
}

type DentryResolver struct {
	pathnames eprobe.Table
}

// Resolve the pathname of a dentry, starting at the pathnameKey in the pathnames table
func (dr *DentryResolver) resolve(pathnameKey uint32) (string, error) {
	// Don't resolve path if pathnameKey isn't valid
	if pathnameKey <= 0 {
		return "", fmt.Errorf("invalid pathname key %v", pathnameKey)
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
		pathRaw, err1 = dr.pathnames.Get(key)
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
		if err2 = dr.pathnames.Delete(key); err2 != nil {
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

// Resolve the pathname of a dentry, starting at the pathnameKey in the pathnames table
func (dr *DentryResolver) Resolve(pathnameKey uint32) string {
	path, _ := dr.resolve(pathnameKey)
	return path
}

func NewDentryResolver(probe *eprobe.Probe) (*DentryResolver, error) {
	pathnames := probe.Table("pathnames")
	if pathnames == nil {
		return nil, fmt.Errorf("pathnames BPF_HASH table doesn't exist")
	}

	return &DentryResolver{
		pathnames: pathnames,
	}, nil
}
