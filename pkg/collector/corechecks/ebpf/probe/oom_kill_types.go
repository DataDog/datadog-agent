package probe

// OOMKillStats contains the statistics of a given socket
type OOMKillStats struct {
	ContainerID string `json:"containerid"`
	Pid         uint32 `json:"pid"`
	TPid        uint32 `json:"tpid"`
	FComm       string `json:"fcomm"`
	TComm       string `json:"tcomm"`
	Pages       uint64 `json:"pages"`
	MemCgOOM    uint32 `json:"memcgoom"`
}
