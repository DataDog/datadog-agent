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

type PathKey struct {
	inode uint64
	dev   uint32
}

func (p *PathKey) Write(buffer []byte) {
	byteOrder.PutUint64(buffer[0:8], p.inode)
	byteOrder.PutUint32(buffer[8:12], p.dev)
	byteOrder.PutUint32(buffer[12:16], 0)
}

func (p *PathKey) Read(buffer []byte) {
	p.inode = byteOrder.Uint64(buffer[0:8])
	p.dev = byteOrder.Uint32(buffer[8:12])
}

func (p *PathKey) IsNull() bool {
	return p.inode == 0 && p.dev == 0
}

func (p *PathKey) String() string {
	return fmt.Sprintf("%x/%x", p.dev, p.inode)
}

type PathValue struct {
	parent PathKey
	name   [256]byte
}

// Resolve the pathname of a dentry, starting at the pathnameKey in the pathnames table
func (dr *DentryResolver) resolve(dev uint32, inode uint64) (filename string, err error) {
	// Don't resolve path if pathnameKey isn't valid
	key := PathKey{dev: dev, inode: inode}
	if key.IsNull() {
		return "", fmt.Errorf("invalid inode/dev couple: %s", key)
	}

	keyBuffer := make([]byte, 16)
	key.Write(keyBuffer)
	done := false
	pathRaw := []byte{}
	var path PathValue

	// Fetch path recursively
	for !done {
		if pathRaw, err = dr.pathnames.Get(keyBuffer); err != nil {
			filename = "*ERROR*" + filename
			break
		}

		path.parent.Read(pathRaw)
		if err = binary.Read(bytes.NewBuffer(pathRaw[16:]), byteOrder, &path.name); err != nil {
			err = errors.Wrap(err, "failed to decode received data (pathLeaf)")
			break
		}

		// Don't append dentry name if this is the root dentry (i.d. name == '/')
		if path.name[0] != '/' {
			filename = "/" + C.GoString((*C.char)(unsafe.Pointer(&path.name))) + filename
		}

		if path.parent.inode == 0 {
			break
		}

		// Prepare next key
		path.parent.Write(keyBuffer)
	}

	if len(filename) == 0 {
		filename = "/"
	}

	return
}

// Resolve the pathname of a dentry, starting at the pathnameKey in the pathnames table
func (dr *DentryResolver) Resolve(dev uint32, inode uint64) string {
	path, _ := dr.resolve(dev, inode)
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
