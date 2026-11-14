// Package protocol provides the experimental low-level SMB2/3 protocol API.
// Its exported API has no compatibility guarantee and may change or be
// removed as the parent client evolves.
//
// https://msdn.microsoft.com/en-us/library/cc246482.aspx
//
// This package doesn't support CAP_UNIX extension.
// Symlink is supported by FSCTL_SET_REPARSE_POINT and FSCTL_GET_REPARSE_POINT.
// The symlink-following algorithm is explained in 2.2.2.2.1 and 2.2.2.2.1.1.
//
// https://msdn.microsoft.com/en-us/library/cc246542.aspx
//
// Supported features and protocol versions are declared in feature.go.
package protocol

import (
	"encoding/binary"
	"io"
	"log"
	"os"
)

var debug = os.Getenv("DEBUG") != ""

var zero [16]byte

var be = binary.BigEndian

var logger *log.Logger

func init() {
	if debug {
		logger = log.New(os.Stderr, "smb2: ", log.LstdFlags)
	} else {
		logger = log.New(io.Discard, "smb2: ", log.LstdFlags)
	}
}
