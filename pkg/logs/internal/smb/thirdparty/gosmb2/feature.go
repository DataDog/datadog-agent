package smb2

import "time"

const (
	clientMaxShareResponseSize = 1024 * 1024
	clientMinBufSize           = 1024
	clientMaxCopyChunkSize     = 1024 * 1024
	clientMaxCopyTotalSize     = 16 * 1024 * 1024
	clientCleanupTimeout       = 5 * time.Second
)
