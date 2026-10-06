package protocol

// This file collects client-side policy defaults, traversal bounds, buffer sizes,
// and timeouts.
//
// These values are client implementation choices rather than protocol mandates.

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"
)

// client

const (
	// SMB 3.x clients advertise DFS support in NEGOTIATE ([MS-SMB2] 2.2.3).
	clientCapabilities = wire.SMB2_GLOBAL_CAP_DFS | wire.SMB2_GLOBAL_CAP_LARGE_MTU | wire.SMB2_GLOBAL_CAP_ENCRYPTION
)

// client negotiation preferences
//
// Slices below are ordered by client preference (most preferred / strongest first).
// For ciphers, 256-bit algorithms precede 128-bit algorithms, and GCM modes precede CCM.
// For dialects, higher revisions precede lower ones ([MS-SMB2] 3.2.4.2.2).

var (
	clientHashAlgorithms        = []uint16{wire.SHA512}
	clientCiphers               = []Cipher{AES256GCM, AES256CCM, AES128GCM, AES128CCM}
	clientCompressionAlgorithms = []uint16{wire.SMB2_COMPRESSION_ALGORITHM_LZ4}
	clientDialects              = []Dialect{SMB311, SMB302, SMB300, SMB210, SMB202}
)

// client timeouts

const (
	clientCreditTimeout       = 30 * time.Second
	clientWriteTimeout        = 30 * time.Second
	clientPacketReadTimeout   = 30 * time.Second
	clientSessionCloseTimeout = 5 * time.Second
	clientQUICKeepAlivePeriod = 15 * time.Second
)

// client concurrency & pipeline limits

const (
	clientMaxCreditBalance = 128
	clientIOPipelineDepth  = 4
)

// client path resolution & traversal & receiver limits

const (
	clientMaxSymlinkDepth = 8
)

// client buffer/chunk size

const (
	clientMinBufSize = 1024
)
