package api

import (
	"fmt"
	"net"
	"os"

	tracepb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/davecgh/go-spew/spew"
)

type DatagramTelemetryForwarder struct {
}

func NewDatagramTelemetryForwarder() *DatagramTelemetryForwarder {
	return &DatagramTelemetryForwarder{}
}

func (f *DatagramTelemetryForwarder) Start() error {
	path := "/var/run/datadog/telemetry.sock"
	fi, err := os.Stat(path)
	if err == nil {
		if fi.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("cannot reuse %q; not a unix socket", path)
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("unable to remove stale socket: %v", err)
		}
	}
	ln, err := net.ListenPacket("unixgram", path)
	if err != nil {
		return err
	}
	if err := os.Chmod(path, 0o722); err != nil {
		return fmt.Errorf("error setting socket permissions: %v", err)
	}
	go func() {
		defer ln.Close()
		for {
			var buf [4096]byte
			n, _, err := ln.ReadFrom(buf[:])
			if err != nil {
				log.Debugf("Failed to read: %v", err)
			}
			bs := buf[:n]
			var t tracepb.Telemetry
			// deserialize
			t.UnmarshalVT(bs)
			// TODO: Translate to telemetry json and forward.
			log.Infof("Got telemetry payload of %d bytes\n", len(bs))
			log.Infof("Payload: %s", spew.Sdump(t))

		}
	}()
	return nil
}
