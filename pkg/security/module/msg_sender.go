// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package module holds module related files
package module

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/reporter"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// MsgSender defines a message sender
type MsgSender interface {
	Send(msg *api.SecurityEventMessage, expireFnc func(*api.SecurityEventMessage))
}

// ChanMsgSender defines a chan message sender
type ChanMsgSender struct {
	msgs chan *api.SecurityEventMessage
}

// Send the message
func (cs *ChanMsgSender) Send(msg *api.SecurityEventMessage, expireFnc func(*api.SecurityEventMessage)) {
	select {
	case cs.msgs <- msg:
		break
	default:
		// The channel is full, consume the oldest event
		select {
		case oldestMsg := <-cs.msgs:
			expireFnc(oldestMsg)
		default:
			break
		}

		// Try to send the event again
		select {
		case cs.msgs <- msg:
			break
		default:
			// Looks like the channel is full again, expire the current message too
			expireFnc(msg)
			break
		}
		break
	}
}

// NewChanMsgSender returns a new chan sender
func NewChanMsgSender(msgs chan *api.SecurityEventMessage) *ChanMsgSender {
	return &ChanMsgSender{
		msgs: msgs,
	}
}

// DirectMsgSender defines a direct sender
type DirectMsgSender struct {
	reporter common.RawReporter
}

// Send the message
func (ds *DirectMsgSender) Send(msg *api.SecurityEventMessage, _ func(*api.SecurityEventMessage)) {
	ds.reporter.ReportRaw(msg.Data, msg.Service, msg.Tags...)
}

// NewDirectMsgSender returns a new direct sender
func NewDirectMsgSender(stopper startstop.Stopper) (*DirectMsgSender, error) {
	useSecRuntimeTrack := pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.use_secruntime_track")

	endpoints, destinationsCtx, err := common.NewLogContextRuntime(useSecRuntimeTrack)
	if err != nil {
		return nil, fmt.Errorf("failed to create direct reported endpoints: %w", err)
	}

	for _, status := range endpoints.GetStatus() {
		log.Info(status)
	}

	// we set the hostname to the empty string to take advantage of the out of the box message hostname
	// resolution
	reporter, err := reporter.NewCWSReporter("", stopper, endpoints, destinationsCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to create direct reporter: %w", err)
	}

	return &DirectMsgSender{
		reporter: reporter,
	}, nil
}

type DiskSender struct {
	outputDir string
}

func NewDiskSender(outputDir string) (*DiskSender, error) {
	if _, err := os.Stat(outputDir); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return &DiskSender{
		outputDir: outputDir,
	}, nil
}

func (ds *DiskSender) Send(msg *api.SecurityEventMessage, _ func(*api.SecurityEventMessage)) {
	fileName := fmt.Sprintf("%s", time.Now().Format(time.RFC3339Nano))
	filePath := ""

	// append event type to output file name
	eventType := ""
	if slices.ContainsFunc(msg.Tags, func(tag string) bool {
		if strings.HasPrefix(tag, "type:") {
			eventType = strings.TrimPrefix(tag, "type:")
			return true
		}
		return false
	}) {
		fileName += "_" + eventType + ".json"
	} else {
		fileName += ".json"
	}

	// prepend containerID if any to output path
	containerID := ""
	if slices.ContainsFunc(msg.Tags, func(tag string) bool {
		if strings.HasPrefix(tag, "container_id:") {
			containerID = strings.TrimPrefix(tag, "container_id:")
			return true
		}
		return false
	}) {
		newOutputDir := filepath.Join(ds.outputDir, containerID)
		// create container output dir if doesnt exist
		_, err := os.Stat(newOutputDir)
		if os.IsNotExist(err) {
			err := os.MkdirAll(newOutputDir, 0755)
			if err != nil {
				log.Errorf("Failed to create output directory %s: %w", newOutputDir, err)
				return
			}
		} else if err != nil {
			log.Errorf("Failed to stat output directory %s: %w", newOutputDir, err)
			return
		}
		filePath = filepath.Join(newOutputDir, fileName)
	} else {
		filePath = filepath.Join(ds.outputDir, fileName)
	}

	// dump the event as json file
	err := os.WriteFile(filePath, msg.Data, 0666)
	if err != nil {
		log.Errorf("Failed to log event: %w", err)
	}
}
