package network

import (
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/vishvananda/netns"
)

// PortMapping tracks which ports a pid is listening on
type PortMapping struct {
	procRoot    string
	collectTCP  bool
	collectIPv6 bool
	ports       map[string]interface{}
	sync.RWMutex
}

//NewPortMapping creates a new PortMapping instance
func NewPortMapping(procRoot string, collectTCP, collectIPv6 bool) *PortMapping {
	return &PortMapping{
		procRoot:    procRoot,
		collectTCP:  collectTCP,
		collectIPv6: collectIPv6,
		ports:       make(map[string]interface{}),
	}
}

// AddMapping indicates that something is listening on the provided port
func (pm *PortMapping) AddMapping(nsIno uint64, port uint16) {
	pm.Lock()
	defer pm.Unlock()

	pm.ports[portMappingKey(nsIno, port)] = struct{}{}
}

// RemoveMapping indicates that the provided port is no longer being listened on
func (pm *PortMapping) RemoveMapping(nsIno uint64, port uint16) {
	pm.Lock()
	defer pm.Unlock()

	delete(pm.ports, portMappingKey(nsIno, port))
}

// IsListening returns true if something is listening on the given port
func (pm *PortMapping) IsListening(nsIno uint64, port uint16) bool {
	pm.RLock()
	defer pm.RUnlock()

	_, ok := pm.ports[portMappingKey(nsIno, port)]
	return ok
}

// ReadInitialState reads the /proc filesystem and determines which ports are being listened on
func (pm *PortMapping) ReadInitialState() error {
	pm.Lock()
	defer pm.Unlock()

	start := time.Now()
	defer func() {
		log.Debugf("Read initial pid->port mapping in %s", time.Now().Sub(start))
	}()

	rootNs, err := util.GetRootNetNamespace(pm.procRoot)
	if err != nil {
		log.Errorf("error getting root net ns: %s", err)
		return err
	}

	defer rootNs.Close()

	seen := make(map[string]interface{})

	return util.WithAllProcs(pm.procRoot, func(pid int) error {
		ns, err := util.GetNetNamespaceFromPid(pm.procRoot, pid)
		if err != nil || ns.Equal(netns.None()) {
			ns = rootNs
		}

		defer func() {
			if !ns.Equal(rootNs) {
				ns.Close()
			}
		}()

		if _, ok := seen[ns.UniqueId()]; ok {
			return nil
		}

		seen[ns.UniqueId()] = struct{}{}

		if ports, err := readProcNetListeners(path.Join(pm.procRoot, fmt.Sprintf("%d/net/tcp", pid))); err != nil {
			log.Errorf("error reading tcp state: %s", err)
		} else {
			for _, port := range ports {
				pm.ports[portMappingKeyFromNs(ns, port)] = struct{}{}
			}
		}

		if !pm.collectIPv6 {
			return nil
		}

		if ports, err := readProcNetListeners(path.Join(pm.procRoot, fmt.Sprintf("%d/net/tcp6", pid))); err != nil {
			log.Errorf("error reading tcp6 state: %s", err)
		} else {
			for _, port := range ports {
				pm.ports[portMappingKeyFromNs(ns, port)] = struct{}{}
			}
		}

		return nil
	})
}

// ReadInitialUDPState reads the /proc filesystem and determines which ports are being used as UDP server
func (pm *PortMapping) ReadInitialUDPState() error {
	pm.Lock()
	defer pm.Unlock()

	rootNs, err := util.GetRootNetNamespace(pm.procRoot)
	if err != nil {
		log.Errorf("error getting root net ns: %s", err)
		return err
	}

	defer rootNs.Close()

	seen := make(map[string]interface{})

	return util.WithAllProcs(pm.procRoot, func(pid int) error {
		ns, err := util.GetNetNamespaceFromPid(pm.procRoot, pid)
		if err != nil || ns.Equal(netns.None()) {
			ns = rootNs
		}

		defer func() {
			if !ns.Equal(rootNs) {
				ns.Close()
			}
		}()

		if _, ok := seen[ns.UniqueId()]; ok {
			return nil
		}

		seen[ns.UniqueId()] = struct{}{}

		udpPath := path.Join(pm.procRoot, fmt.Sprintf("%d/net/udp", pid))
		if ports, err := readProcNetWithStatus(udpPath, tcpClose); err != nil {
			log.Errorf("failed to read UDP state for net ns %s: %s", ns, err)
		} else {
			log.Tracef("read UDP ports for net ns %s: %v", ns, ports)
			for _, port := range ports {
				pm.ports[portMappingKeyFromNs(ns, port)] = struct{}{}
			}
		}

		if !pm.collectIPv6 {
			return nil
		}

		if ports, err := readProcNetWithStatus(path.Join(pm.procRoot, fmt.Sprintf("%d/net/udp6", pid)), tcpClose); err != nil {
			log.Errorf("error reading UDPv6 state for net ns %s: %s", ns, err)
		} else {
			log.Tracef("read UDPv6 state for net ns %s: %v", ns, ports)
			for _, port := range ports {
				pm.ports[portMappingKeyFromNs(ns, port)] = struct{}{}
			}
		}

		return nil
	})
}

func portMappingKey(nsIno uint64, port uint16) string {
	return fmt.Sprintf("%d:%d", nsIno, port)
}

func portMappingKeyFromNs(ns netns.NsHandle, port uint16) string {
	ino, _ := util.GetInoForNs(ns)
	return portMappingKey(ino, port)
}
