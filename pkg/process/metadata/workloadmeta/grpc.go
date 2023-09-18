// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"github.com/DataDog/datadog-agent/pkg/config"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DuplicateConnectionErr is an error that explains the connection was closed because another client tried to connect
var DuplicateConnectionErr = errors.New("the stream was closed because another client called StreamEntities")

// GRPCServer implements a gRPC server to expose Process Entities collected with a WorkloadMetaExtractor
type GRPCServer struct {
	config    config.ConfigReader
	extractor *WorkloadMetaExtractor
	server    *grpc.Server
	// The address of the server set by start(). Primarily used for testing. May be nil if start() has not been called.
	addr net.Addr

	wg sync.WaitGroup

	streamMutex         *sync.Mutex
	closeExistingStream context.CancelFunc
}

const keepaliveInterval = 10 * time.Second
const streamSendTimeout = 1 * time.Minute

var (
	invalidVersionError = telemetry.NewSimpleCounter(subsystem, "invalid_version_errors", "The number of times the grpc server receives an entity diff that has an invalid version.")
	streamServerError   = telemetry.NewSimpleCounter(subsystem, "stream_send_errors", "The number of times the grpc server has failed to send an entity diff to the core agent.")
)

// NewGRPCServer creates a new instance of a GRPCServer
func NewGRPCServer(config config.ConfigReader, extractor *WorkloadMetaExtractor) *GRPCServer {
	l := &GRPCServer{
		config:    config,
		extractor: extractor,
		server: grpc.NewServer(
			grpc.Creds(insecure.NewCredentials()),
			grpc.KeepaliveParams(keepalive.ServerParameters{
				Time: keepaliveInterval,
			}),
		),
		streamMutex: &sync.Mutex{},
	}

	pbgo.RegisterProcessEntityStreamServer(l.server, l)
	return l
}

func (l *GRPCServer) consumeProcessDiff(diff *ProcessCacheDiff) ([]*pbgo.ProcessEventSet, []*pbgo.ProcessEventUnset) {
	setEvents := make([]*pbgo.ProcessEventSet, len(diff.creation))
	for i, proc := range diff.creation {
		setEvents[i] = processEntityToEventSet(proc)
	}

	unsetEvents := make([]*pbgo.ProcessEventUnset, len(diff.deletion))
	for i, proc := range diff.deletion {
		unsetEvents[i] = &pbgo.ProcessEventUnset{Pid: proc.Pid}
	}

	return setEvents, unsetEvents
}

// Start starts the GRPCServer to listen for new connections
func (l *GRPCServer) Start() error {
	log.Info("Starting Process Entity WorkloadMeta gRPC server")
	listener, err := getListener(l.config)
	if err != nil {
		return err
	}
	l.addr = listener.Addr()
	log.Info("Process Entity WorkloadMeta gRPC server has started listening on", listener.Addr().String())

	l.wg.Add(1)
	go func() {
		defer l.wg.Done()

		err = l.server.Serve(listener)
		if err != nil {
			log.Error(err)
		}
	}()

	return nil
}

func (l *GRPCServer) Addr() net.Addr {
	return l.addr
}

// Stop stops and cleans up resources allocated by the GRPCServer
func (l *GRPCServer) Stop() {
	log.Info("Stopping Process Entity WorkloadMeta gRPC server")
	l.server.Stop()
	l.wg.Wait()
	log.Info("Process Entity WorkloadMeta gRPC server stopped")
}

func sendMsg(stream pbgo.ProcessEntityStream_StreamEntitiesServer, msg *pbgo.ProcessStreamResponse) error {
	return grpcutil.DoWithTimeout(func() error {
		return stream.Send(msg)
	}, streamSendTimeout)
}

// StreamEntities streams Process Entities collected through the WorkloadMetaExtractor
func (l *GRPCServer) StreamEntities(_ *pbgo.ProcessStreamEntitiesRequest, out pbgo.ProcessEntityStream_StreamEntitiesServer) error {
	streamCtx := l.acquireStreamCtx()

	// When connection is created, send a snapshot of all processes detected on the host so far as "SET" events
	procs, snapshotVersion := l.extractor.GetAllProcessEntities()
	setEvents := make([]*pbgo.ProcessEventSet, 0, len(procs))
	for _, proc := range procs {
		setEvents = append(setEvents, processEntityToEventSet(proc))
	}

	syncMessage := &pbgo.ProcessStreamResponse{
		EventID:   snapshotVersion,
		SetEvents: setEvents,
	}
	err := sendMsg(out, syncMessage)
	if err != nil {
		streamServerError.Inc()
		log.Warnf("error sending process entity event: %s", err)
		return err
	}

	expectedVersion := snapshotVersion + 1
	// Once connection is established, only diffs (process creations/deletions) are sent to the client
	for {
		select {
		case diff := <-l.extractor.ProcessCacheDiff():
			// Ensure that if streamCtx.Done() is closed, we always choose that path.
			select {
			case <-streamCtx.Done():
				return DuplicateConnectionErr
			default:
			}

			// Do not send diff if it has the same or older version of the cache snapshot sent on the connection creation
			if diff.cacheVersion <= snapshotVersion {
				continue
			}

			// The diff received from the channel should be 1 + the previous version. Otherwise, we have lost data,
			// and we should signal the client to resync by closing the stream.
			log.Trace("[WorkloadMeta GRPCServer] expected diff version %d, actual %d", expectedVersion, diff.cacheVersion)
			if diff.cacheVersion != expectedVersion {
				invalidVersionError.Inc()
				log.Debug("[WorkloadMeta GRPCServer] missing cache diff - dropping stream")
				return fmt.Errorf("missing cache diff: received version = %d; expected = %d", diff.cacheVersion, expectedVersion)
			}
			expectedVersion++

			sets, unsets := l.consumeProcessDiff(diff)
			msg := &pbgo.ProcessStreamResponse{
				EventID:     diff.cacheVersion,
				SetEvents:   sets,
				UnsetEvents: unsets,
			}
			err := sendMsg(out, msg)
			if err != nil {
				streamServerError.Inc()
				log.Warnf("error sending process entity event: %s", err)
				return err
			}

		case <-out.Context().Done():
			err := out.Context().Err()
			if err != nil {
				log.Warn("The workloadmeta grpc stream was closed:", err)
			}
			return nil
		case <-streamCtx.Done():
			return DuplicateConnectionErr
		}
	}
}

// getListener returns a listening connection
func getListener(cfg config.ConfigReader) (net.Listener, error) {
	host, err := config.GetIPCAddress()
	if err != nil {
		return nil, err
	}

	address := net.JoinHostPort(host, strconv.Itoa(getGRPCStreamPort(cfg)))
	return net.Listen("tcp", address)
}

func getGRPCStreamPort(cfg config.ConfigReader) int {
	grpcPort := cfg.GetInt("process_config.language_detection.grpc_port")
	if grpcPort <= 0 {
		log.Warnf("Invalid process_config.language_detection.grpc_port -- %d, using default port %d", grpcPort, config.DefaultProcessEntityStreamPort)
		grpcPort = config.DefaultProcessEntityStreamPort
	}
	return grpcPort
}

func processEntityToEventSet(proc *ProcessEntity) *pbgo.ProcessEventSet {
	var language *pbgo.Language
	if proc.Language != nil {
		language = &pbgo.Language{Name: string(proc.Language.Name)}
	}

	return &pbgo.ProcessEventSet{
		Pid:          proc.Pid,
		ContainerId:  proc.ContainerId,
		Nspid:        proc.NsPid,
		CreationTime: proc.CreationTime,
		Language:     language,
	}
}

// acquireStreamCtx is responsible for handling locking and cancelling running streams. This ensures that whenever a
// new client connects, the stream is unique.
func (l *GRPCServer) acquireStreamCtx() context.Context {
	l.streamMutex.Lock()
	defer l.streamMutex.Unlock()

	if l.closeExistingStream != nil {
		l.closeExistingStream()
	}
	streamCtx, cancel := context.WithCancel(context.Background())
	l.closeExistingStream = cancel
	return streamCtx
}
