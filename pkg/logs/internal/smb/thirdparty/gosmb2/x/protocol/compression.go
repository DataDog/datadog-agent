package protocol

import (
	"errors"
	"fmt"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"
	"github.com/pierrec/lz4/v4"
)

const compressionHeaderSize = 16

func compressPacket(pkt []byte) ([]byte, error) {
	if len(pkt) == 0 {
		return pkt, nil
	}
	return compressPacketInto(pkt, make([]byte, maxCompressedPacketSize(len(pkt))))
}

func maxCompressedPacketSize(size int) int {
	return compressionHeaderSize + lz4.CompressBlockBound(size)
}

func compressPacketInto(pkt, dst []byte) ([]byte, error) {
	if len(pkt) == 0 {
		return pkt, nil
	}
	if len(dst) < maxCompressedPacketSize(len(pkt)) {
		return nil, fmt.Errorf("compression buffer is too small")
	}

	compressed := dst[compressionHeaderSize:]
	var compressor lz4.Compressor
	n, err := compressor.CompressBlock(pkt, compressed)
	if err != nil {
		return nil, err
	}
	// CompressBlock returns zero for data which is not compressible.
	// SMB2 requires the original packet in that case.
	if n == 0 || n >= len(pkt) {
		return pkt, nil
	}

	result := dst[:compressionHeaderSize+n]
	c := wire.CompressionCodec(result)
	c.SetProtocolId()
	c.SetOriginalCompressedSegmentSize(uint32(len(pkt)))
	c.SetCompressionAlgorithm(wire.SMB2_COMPRESSION_ALGORITHM_LZ4)
	c.SetFlags(wire.SMB2_COMPRESSION_FLAG_NONE)
	c.SetOffset(0)
	return result, nil
}

func (conn *conn) compressionEnabled() bool {
	if conn == nil || conn.dialect != wire.SMB311 {
		return false
	}
	return slices.Contains(conn.compressionIds, wire.SMB2_COMPRESSION_ALGORITHM_LZ4)
}

func decompressPacket(conn *conn, pkt []byte) ([]byte, error) {
	output, _, err := decompressPacketForReceive(conn, pkt, nil)
	return output, err
}

func decompressPacketForReceive(conn *conn, pkt []byte, findSink directSinkFinder) ([]byte, []byte, error) {
	if conn == nil || !conn.compressionEnabled() {
		return nil, nil, &InvalidResponseError{Message: "compression was not negotiated"}
	}

	c := wire.CompressionCodec(pkt)
	if c.IsInvalid() {
		return nil, nil, &InvalidResponseError{Message: "broken compression header format"}
	}
	if c.CompressionAlgorithm() != wire.SMB2_COMPRESSION_ALGORITHM_LZ4 {
		return nil, nil, &InvalidResponseError{Message: "unsupported compression algorithm"}
	}
	if c.Flags() != wire.SMB2_COMPRESSION_FLAG_NONE {
		return nil, nil, &InvalidResponseError{Message: "chained compression is not supported"}
	}

	originalSize := uint64(c.OriginalCompressedSegmentSize())
	offset := uint64(c.Offset())
	largestMessage := uint64(max(conn.maxReadSize, max(conn.maxWriteSize, conn.maxTransactSize)))
	if originalSize > 256+compressionHeaderSize+largestMessage {
		return nil, nil, &InvalidResponseError{Message: "compressed segment is too large"}
	}
	if offset > uint64(len(pkt)-compressionHeaderSize) {
		return nil, nil, &InvalidResponseError{Message: "compression offset exceeds packet"}
	}

	fullSize := offset + originalSize
	if fullSize < offset || fullSize > maxDirectTCPSize || fullSize > uint64(int(^uint(0)>>1)) {
		return nil, nil, &InvalidResponseError{Message: "decompressed packet is too large"}
	}

	prefixEnd := compressionHeaderSize + int(offset)
	prefix := pkt[compressionHeaderSize:prefixEnd]
	compressed := pkt[prefixEnd:]
	decompress := func(dst []byte) error {
		n, err := lz4.UncompressBlock(compressed, dst)
		if err != nil {
			return err
		}
		if uint64(n) != originalSize {
			return errors.New("decompressed segment size mismatch")
		}
		return nil
	}

	// [MS-SMB2] 3.1.4.4 permits an uncompressed prefix. If it exposes a
	// complete standalone READ header and the compressed segment is exactly
	// the payload, expand directly into the caller's registered buffer.
	if findSink != nil && offset >= 80 && originalSize > 0 {
		if sink, frontSize := findSink(prefix, int(fullSize)-80); sink != nil &&
			frontSize == int(offset) && len(sink) == int(originalSize) {
			if err := decompress(sink); err != nil {
				return nil, nil, err
			}
			return prefix, sink, nil
		}
	}

	output := make([]byte, int(fullSize))
	copy(output[:int(offset)], prefix)
	if err := decompress(output[int(offset):]); err != nil {
		return nil, nil, err
	}
	if wire.PacketCodec(output).IsInvalid() {
		return nil, nil, &InvalidResponseError{Message: "broken decompressed packet format"}
	}
	return output, nil, nil
}
