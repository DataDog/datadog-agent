// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiimpl

import (
	"crypto/tls"
	"fmt"
	stdLog "log"
	"net"
	"net/http"

	"github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/gui"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	dogstatsddebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcserviceha"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

func startServer(listener net.Listener, srv *http.Server, name string) {
	// Use a stack depth of 4 on top of the default one to get a relevant filename in the stdlib
	logWriter, _ := config.NewLogWriter(5, seelog.ErrorLvl)

	srv.ErrorLog = stdLog.New(logWriter, fmt.Sprintf("Error from the Agent HTTP server '%s': ", name), 0) // log errors to seelog

	tlsListener := tls.NewListener(listener, srv.TLSConfig)

	go srv.Serve(tlsListener) //nolint:errcheck

	log.Infof("Started HTTP server '%s' on %s", name, listener.Addr().String())
}

func stopServer(listener net.Listener, name string) {
	if listener != nil {
		if err := listener.Close(); err != nil {
			log.Errorf("Error stopping HTTP server '%s': %s", name, err)
		} else {
			log.Infof("Stopped HTTP server '%s'", name)
		}
	}
}

// StartServers creates certificates and starts API + IPC servers
func StartServers(
	configService optional.Option[rcservice.Component],
	configServiceHA optional.Option[rcserviceha.Component],
	flare flare.Component,
	dogstatsdServer dogstatsdServer.Component,
	capture replay.Component,
	pidMap pidmap.Component,
	serverDebug dogstatsddebug.Component,
	wmeta workloadmeta.Component,
	taggerComp tagger.Component,
	logsAgent optional.Option[logsAgent.Component],
	senderManager sender.DiagnoseSenderManager,
	hostMetadata host.Component,
	invAgent inventoryagent.Component,
	demux demultiplexer.Component,
	invHost inventoryhost.Component,
	secretResolver secrets.Component,
	invChecks inventorychecks.Component,
	pkgSigning packagesigning.Component,
	statusComponent status.Component,
	collector optional.Option[collector.Component],
	eventPlatformReceiver eventplatformreceiver.Component,
	ac autodiscovery.Component,
	gui optional.Option[gui.Component],
) error {
	apiAddr, err := getIPCAddressPort()
	if err != nil {
		return fmt.Errorf("unable to get IPC address and port: %v", err)
	}

	additionalHostIdentities := []string{apiAddr}

	ipcServerHost, ipcServerHostPort, ipcServerEnabled := getIPCServerAddressPort()
	if ipcServerEnabled {
		additionalHostIdentities = append(additionalHostIdentities, ipcServerHost)
	}

	tlsKeyPair, tlsCertPool, err := initializeTLS(additionalHostIdentities...)
	if err != nil {
		return fmt.Errorf("unable to initialize TLS: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*tlsKeyPair},
		NextProtos:   []string{"h2"},
		MinVersion:   tls.VersionTLS12,
	}

	// start the CMD server
	if err := startCMDServer(
		apiAddr,
		tlsConfig,
		tlsCertPool,
		configService,
		configServiceHA,
		flare,
		dogstatsdServer,
		capture,
		pidMap,
		serverDebug,
		wmeta,
		taggerComp,
		logsAgent,
		senderManager,
		hostMetadata,
		invAgent,
		demux,
		invHost,
		secretResolver,
		invChecks,
		pkgSigning,
		statusComponent,
		collector,
		eventPlatformReceiver,
		ac,
		gui,
	); err != nil {
		return fmt.Errorf("unable to start CMD API server: %v", err)
	}

	// start the IPC server
	if ipcServerEnabled {
		if err := startIPCServer(ipcServerHostPort, tlsConfig); err != nil {
			// if we fail to start the IPC server, we should stop the CMD server
			StopServers()
			return fmt.Errorf("unable to start IPC API server: %v", err)
		}
	}

	return nil
}

// StopServers closes the connections and the servers
func StopServers() {
	stopCMDServer()
	stopIPCServer()
}
