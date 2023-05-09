/*
 * GOPACP - PCAP file parsing in Golang
 * Copyright (c) 2017 Philipp Mieden <dreadl0ck [at] protonmail [dot] ch>
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package gopcap

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/davecgh/go-spew/spew"
)

/////////////////////////////
// Reader
/////////////////////////////

// Reader struct
type Reader struct {
	FileHandle *os.File
	Buffer     *bufio.Reader
	Header     FileHeader
}

// Open pcap file
func Open(filename string) (*Reader, error) {

	var (
		r   = &Reader{}
		err error
	)

	r.FileHandle, err = os.Open(filename)
	if err != nil {
		return nil, err
	}

	var buff [24]byte
	if _, err := io.ReadFull(r.FileHandle, buff[:]); err != nil {
		return nil, err
	}

	r.Header = FileHeader{
		MagicNumber:  binary.LittleEndian.Uint32(buff[:4]),
		VersionMajor: binary.LittleEndian.Uint16(buff[4:6]),
		VersionMinor: binary.LittleEndian.Uint16(buff[6:8]),
		Thiszone:     int32(binary.LittleEndian.Uint32(buff[8:12])),
		Sigfigs:      binary.LittleEndian.Uint32(buff[12:16]),
		Snaplen:      binary.LittleEndian.Uint32(buff[16:20]),
		Network:      binary.LittleEndian.Uint32(buff[20:24]),
	}

	r.Buffer = bufio.NewReader(r.FileHandle)
	return r, nil
}

// ReadNextPacket reads the next packet. returns header,data,error
func (r *Reader) ReadNextPacket() (PacketHeader, []byte, error) {

	var buff [16]byte
	if _, err := io.ReadFull(r.Buffer, buff[:]); err != nil {
		return PacketHeader{}, nil, err
	}

	pcaprecHdr := PacketHeader{
		TsSec:       int32(binary.LittleEndian.Uint32(buff[0:4])),
		TsUsec:      int32(binary.LittleEndian.Uint32(buff[4:8])),
		CaptureLen:  int32(binary.LittleEndian.Uint32(buff[8:12])),
		OriginalLen: int32(binary.LittleEndian.Uint32(buff[12:16])),
	}

	var buf bytes.Buffer
	if pcaprecHdr.CaptureLen < 1 {
		fmt.Println("invalid pcaprecHdr.CaptureLen:", pcaprecHdr.CaptureLen)
		spew.Dump(pcaprecHdr)
		spew.Dump(buff)
		panic("invalid capture length")
	} else {
		if _, err := io.Copy(&buf, r.Buffer); err != nil {
			return pcaprecHdr, buf.Bytes(), err
		}
	}

	return pcaprecHdr, buf.Bytes(), nil
}

// ReadNextPacketHeader read next packet header. returns header,data,error
// @TODO: add a bytes.Reader after the buffered reader, and seek inside that buffer instead of reading all the bytes.
func (r *Reader) ReadNextPacketHeader() (PacketHeader, []byte, error) {

	var buff [16]byte
	if _, err := io.ReadFull(r.Buffer, buff[:]); err != nil {
		return PacketHeader{}, nil, err
	}

	pcaprecHdr := PacketHeader{
		TsSec:       int32(binary.LittleEndian.Uint32(buff[0:4])),
		TsUsec:      int32(binary.LittleEndian.Uint32(buff[4:8])),
		CaptureLen:  int32(binary.LittleEndian.Uint32(buff[8:12])),
		OriginalLen: int32(binary.LittleEndian.Uint32(buff[12:16])),
	}

	var buf = make([]byte, pcaprecHdr.CaptureLen)
	if _, err := io.ReadFull(r.Buffer, buf); err != nil {
		return pcaprecHdr, buf, err
	}

	return pcaprecHdr, buf, nil
}

// Close pcap file
func (r *Reader) Close() error {
	return r.FileHandle.Close()
}
