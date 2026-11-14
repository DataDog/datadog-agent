package smb2

import (
	"context"
	"encoding/binary"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/protocol"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"
)

func (fs *Share) negotiateAAPL(ctx context.Context) error {
	// Query server capabilities and advertise NFS ACE support. The standard
	// decoder does not support the READ_DIR_ATTR directory entry format.
	query := protocol.AaplExtensionContext{
		Command:            protocol.AAPL_SERVER_QUERY,
		RequestBitmap:      protocol.AAPL_SERVER_CAPS,
		ClientCapabilities: protocol.AAPL_SUPPORTS_NFS_ACE,
	}
	res, err := fs.Request().Create("", wire.READ_CONTROL, wire.FILE_OPEN,
		wire.FILE_DIRECTORY_FILE, 0, query).Close().Do(ctx)
	if err != nil {
		// A transport failure leaves the tree unusable even though AAPL is optional.
		if _, ok := errors.AsType[*protocol.TransportError](err); ok {
			return err
		}
		return nil // AAPL is optional; an unsupported server can reject the context.
	}
	defer res.Close()

	create, err := res.Create(0)
	if err != nil || create.CreateContextsLength() == 0 {
		return nil
	}
	for _, entry := range create.Contexts().Contexts() {
		nameOffset := int(binary.LittleEndian.Uint16(entry[4:6]))
		nameLength := int(binary.LittleEndian.Uint16(entry[6:8]))
		if nameLength != 4 || string(entry[nameOffset:nameOffset+nameLength]) != "AAPL" {
			continue
		}
		dataOffset := int(binary.LittleEndian.Uint16(entry[10:12]))
		dataLength := int(binary.LittleEndian.Uint32(entry[12:16]))
		if dataLength < 24 {
			return nil
		}
		data := entry[dataOffset : dataOffset+dataLength]
		if binary.LittleEndian.Uint32(data[:4]) != protocol.AAPL_SERVER_QUERY ||
			binary.LittleEndian.Uint64(data[8:16])&protocol.AAPL_SERVER_CAPS == 0 {
			return nil
		}
		fs.aaplCapabilities = binary.LittleEndian.Uint64(data[16:24])
		return nil
	}
	return nil
}
