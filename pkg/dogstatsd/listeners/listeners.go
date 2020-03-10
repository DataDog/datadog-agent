package listeners

import "github.com/DataDog/datadog-agent/pkg/config"

// GlobalPacketPool is used by the packet assembler to retrieve already allocated
// buffer in order to avoid allocation.
var GlobalPacketPool *PacketPool = NewPacketPool(config.Datadog.GetInt("dogstatsd_buffer_size"))
