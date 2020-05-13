package probe

import (
	"C"

	"bytes"
	"encoding/binary"
	"fmt"
	"unsafe"

	"github.com/pkg/errors"

	eprobe "github.com/DataDog/datadog-agent/pkg/ebpf/probe"
)

type DentryResolver struct {
	pathnames eprobe.Table
}

// Resolve the pathname of a dentry, starting at the pathnameKey in the pathnames table
func (dr *DentryResolver) resolve(pathnameKey uint64) (filename string, err error) {
	// Don't resolve path if pathnameKey isn't valid
	if pathnameKey <= 0 {
		return "", fmt.Errorf("invalid pathname key %v", pathnameKey)
	}

	// Convert key into bytes
	key := make([]byte, 8)
	byteOrder.PutUint64(key, pathnameKey)
	done := false
	pathRaw := []byte{}
	var path struct {
		ParentKey uint64
		Name      [256]byte
	}

	// Fetch path recursively
	for !done {
		if pathRaw, err = dr.pathnames.Get(key); err != nil {
			filename = "*ERROR*" + filename
			break
		}

		path.ParentKey = byteOrder.Uint64(pathRaw[0:8])
		if err = binary.Read(bytes.NewBuffer(pathRaw[8:]), byteOrder, &path.Name); err != nil {
			err = errors.Wrap(err, "failed to decode received data (pathLeaf)")
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
		byteOrder.PutUint64(key, path.ParentKey)
	}

	if len(filename) == 0 {
		filename = "/"
	}

	return
}

// Resolve the pathname of a dentry, starting at the pathnameKey in the pathnames table
func (dr *DentryResolver) Resolve(pathnameKey uint64) string {
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
