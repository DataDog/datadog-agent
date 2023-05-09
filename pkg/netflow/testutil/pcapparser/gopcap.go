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

/////////////////////////////
// Data Structures
/////////////////////////////

// FileHeader global PCAP file header
// for more info: https://wiki.wireshark.org/Development/LibpcapFileFormat
type FileHeader struct {
	// magic number
	MagicNumber uint32 // 0
	// major version number
	VersionMajor uint16 // 4
	// minor version number
	VersionMinor uint16 // 6
	// GMT to local correction
	Thiszone int32 // 8
	// accuracy of timestamps
	Sigfigs uint32 // 12
	// max length of captured packets, in octets
	Snaplen uint32 // 16
	// data link type
	Network uint32 // 20
} // 24

// PacketHeader is a PCAP packet header
type PacketHeader struct {
	// timestamp seconds
	TsSec int32 // 0
	// timestamp microseconds
	TsUsec int32 // 4
	// number of octets of packet saved in file
	CaptureLen int32 // 8
	// actual length of packet
	OriginalLen int32 // 12
} // 16
