// Code generated by cmd/cgo -godefs; DO NOT EDIT.
// cgo -godefs -- -I ../../ebpf/c -I ../../../ebpf/c -fsigned-char types.go

package http

type ConnTuple = struct {
	Saddr_h  uint64
	Saddr_l  uint64
	Daddr_h  uint64
	Daddr_l  uint64
	Sport    uint16
	Dport    uint16
	Netns    uint32
	Pid      uint32
	Metadata uint32
}
type SslSock struct {
	Tup       ConnTuple
	Fd        uint32
	Pad_cgo_0 [4]byte
}
type SslReadArgs struct {
	Ctx uint64
	Buf uint64
}
type SslReadExArgs struct {
	Ctx       uint64
	Buf       uint64
	Out_param uint64
}
type SslWriteArgs struct {
	Ctx uint64
	Buf uint64
}
type SslWriteExArgs struct {
	Ctx       uint64
	Buf       uint64
	Out_param uint64
}

type EbpfEvent struct {
	Tuple ConnTuple
	Http  EbpfTx
}
type EbpfTx struct {
	Request_started      uint64
	Response_last_seen   uint64
	Tags                 uint64
	Tcp_seq              uint32
	Response_status_code uint16
	Request_method       uint8
	Pad_cgo_0            [1]byte
	Request_fragment     [208]byte
}

const (
	BufferSize = 0xd0
)
