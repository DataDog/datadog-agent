package resolver

import (
	"sync"
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

const defaultTTL = 10 * time.Second

// LocalResolver is responsible resolving the raddr of connections when they are local containers
type LocalResolver struct {
	mux         sync.RWMutex
	addrToCtrID map[addr]string
	updated     time.Time
}

type addr struct {
	ip    string
	port  int32
	proto string
}

// LoadAddrs generates a map of network addresses to container IDs
func (l *LocalResolver) LoadAddrs(containers []*containers.Container) {
	l.mux.Lock()
	defer l.mux.Unlock()

	if time.Now().Sub(l.updated) < defaultTTL {
		return
	}

	l.updated = time.Now()
	l.addrToCtrID = make(map[addr]string)
	for _, ctr := range containers {
		for _, networkAddr := range ctr.AddressList {
			addr := addr{
				ip:    networkAddr.IP.String(),
				port:  int32(networkAddr.Port),
				proto: networkAddr.Protocol,
			}
			l.addrToCtrID[addr] = ctr.ID
		}
	}
}

// Resolve binds container IDs to the Raddr of connections
func (l *LocalResolver) Resolve(c *model.Connections) {
	l.mux.RLock()
	defer l.mux.RUnlock()

	for _, conn := range c.Conns {
		raddr := conn.Raddr

		addr := addr{
			ip:    raddr.Ip,
			port:  raddr.Port,
			proto: conn.Type.String(),
		}

		raddr.ContainerId = l.addrToCtrID[addr]
	}
}
