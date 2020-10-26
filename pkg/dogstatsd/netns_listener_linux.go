// +build linux,docker

package dogstatsd

import (
	"io"
	"regexp"
	"runtime"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/vishvananda/netns"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// hostNetwork services run in the default netns that we cannot bind into
var netNsBlacklist = regexp.MustCompile("^.+/docker/netns/default$")

func (s *Server) setupNetNsListeners() {
	d, err := docker.GetDockerUtil()
	if err != nil {
		log.Errorf("Failed to instantiate docker util - %v", err)
		return
	}

	l := &netNsListener{
		server:     s,
		dockerUtil: d,
		services:   make(map[string]*netNsMetadata),
		stop:       make(chan bool),
		health:     health.RegisterLiveness("netns-listener"),
	}

	l.init() // init listeners in containers that are already running
	log.Debugf("Service Map: %+v", l.services)
	log.Debugf("Service Map Len: %d", len(l.services))

	messages, errs, err := l.dockerUtil.SubscribeToContainerEvents("netNsListener")
	if err != nil {
		log.Errorf("can't listen to docker events: %v", err)
		signals.ErrorStopper <- true
		return
	}

	go func() {
		for {
			select {
			case <-l.stop:
				log.Info("Recieved stop signal, shutting down listeners")
				l.dockerUtil.UnsubscribeFromContainerEvents("netNsListener")
				for sID, _ := range l.services {
					l.removeService(sID)
				}
				l.health.Deregister()
				return
			case <-l.health.C:
			case msg := <-messages:
				log.Infof("Processing container event %s", msg.Action)
				l.processEvent(msg)
			case err := <-errs:
				if err != nil && err != io.EOF {
					log.Errorf("docker listener error: %v", err)
					signals.ErrorStopper <- true
				}
				return
			}
		}
	}()
}

// Stop queues a shutdown of netNsListener
func (l *netNsListener) Stop() {
	l.stop <- true
}

func (l *netNsListener) socketListen(nnm *netNsMetadata) {
	udpListener, err := listeners.NewUDPListener(l.server.packetsIn, l.server.sharedPacketPool)
	if err != nil {
		log.Errorf("Cannot listen in a netNS: %v", err)
		log.Debug("Short circuiting and skipping this container, ensuring it is not in service map")
		l.m.Lock()
		delete(l.services, nnm.cID)
		l.m.Unlock()
		return
	}
	udpListener.Origin = "container_id://" + nnm.cID
	nnm.listener = udpListener
	udpListener.Listen()
}

type netNsMetadata struct {
	cID      string
	path     string
	listener *listeners.UDPListener
}

type netNsListener struct {
	server     *Server
	dockerUtil *docker.DockerUtil
	services   map[string]*netNsMetadata
	stop       chan bool
	health     *health.Handle
	m          sync.RWMutex
}

func (l *netNsListener) createService(cID string) {
	var nnm netNsMetadata
	cInspect, err := l.dockerUtil.Inspect(cID, false)
	if err != nil {
		log.Errorf("Failed to inspect container %s - %s", cID[:12], err)
	} else {
		nnm = netNsMetadata{path: cInspect.NetworkSettings.SandboxKey, cID: cID}
	}

	if cInspect.HostConfig.NetworkMode.IsContainer() {
		return
	}

	if netNsBlacklist.Match([]byte(nnm.path)) {
		return
	}

	l.m.Lock()
	l.services[cID] = &nnm
	l.m.Unlock()

	log.Infof("Initializing listener in a new netns %s", nnm.path)
	go executeWithinNetNS(&nnm, l.socketListen)
}

func (l *netNsListener) processEvent(e *docker.ContainerEvent) {
	containers, err := l.dockerUtil.RawContainerList(types.ContainerListOptions{})
	if err != nil {
		log.Errorf("Couldn't retrieve container list - %s", err)
		return
	}
	switch e.Action {
	case "die":
		// Loop service map removing any not in containers
		containerIDs := make(map[string]bool)
		deadIDs := make([]string, 0)
		for _, co := range containers {
			containerIDs[co.ID] = true
		}
		l.m.Lock()
		for sID, _ := range l.services {
			_, ok := containerIDs[sID]
			if !ok {
				deadIDs = append(deadIDs, sID)
			}
		}
		l.m.Unlock()
		for _, dID := range deadIDs {
			log.Infof("Removing for id %s", dID)
			l.removeService(dID)
		}
	case "start":
		for _, co := range containers {
			log.Debugf("Container list ID: %s\tNAMES: %s", co.ID, co.Names)
			l.m.RLock()
			_, found := l.services[co.ID]
			l.m.RUnlock()
			if !found {
				l.createService(co.ID)
			}
		}
	default:
		log.Errorf("Expected die or start event got %s from %s", e.Action, e.ContainerID[:12])
	}
	log.Debugf("Service Map Len: %d", len(l.services))
	log.Debugf("Service Map: %+v", l.services)
}

func (l *netNsListener) removeService(cID string) {
	l.m.RLock()
	svc, ok := l.services[cID]
	l.m.RUnlock()

	if ok {
		log.Infof("Removing listener from a netns %s", svc.path)
		svc.listener.Stop()
		l.m.Lock()
		delete(l.services, cID)
		l.m.Unlock()
	} else {
		log.Errorf("Container %s not found, not removing", cID[:12])
	}
}

func (l *netNsListener) init() {
	l.m.Lock()
	defer l.m.Unlock()

	containers, err := l.dockerUtil.RawContainerList(types.ContainerListOptions{})
	if err != nil {
		log.Errorf("Couldn't retrieve container list - %s", err)
	}

	for _, c := range containers {
		var nnm netNsMetadata
		cInspect, err := l.dockerUtil.Inspect(c.ID, false)
		if err != nil {
			log.Errorf("Failed to inspect container %s - %s", c.ID[:12], err)
		} else {
			nnm = netNsMetadata{path: cInspect.NetworkSettings.SandboxKey, cID: c.ID}
		}

		if cInspect.HostConfig.NetworkMode.IsContainer() {
			continue
		}

		if netNsBlacklist.Match([]byte(nnm.path)) {
			continue
		}

		log.Infof("Initializing listener in existing netns %s", nnm.path)
		l.services[c.ID] = &nnm
		go executeWithinNetNS(&nnm, l.socketListen)
	}
}

// executeWithinNetNS runs code inside a given network namespace, passing metadata
func executeWithinNetNS(nnm *netNsMetadata, cb func(*netNsMetadata)) {
	if nnm.path == "" {
		log.Errorf("Target netNS is empty")
		return
	}

	// confine this gorouting into this OS thread, so it stays in netNS
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	originalNS, err := netns.Get()
	if err != nil {
		log.Errorf("Cannot get current netNS: %w", err)
		return
	}
	defer originalNS.Close()

	targetNS, err := netns.GetFromPath(nnm.path)
	if err != nil {
		log.Errorf("Cannot get target netNS: %w", err)
		return
	}

	// enter target netNS
	if err := netns.Set(targetNS); err != nil {
		log.Errorf("Cannot set target netNS: %w", err)
		return
	}
	// later, return to original netNS
	defer func() {
		err := netns.Set(originalNS)
		if err != nil {
			log.Errorf("Cannot return to original netNS: %v", err)
			return
		}
	}()

	cb(nnm)
}
