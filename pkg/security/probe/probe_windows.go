// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/etw"
	etwimpl "github.com/DataDog/datadog-agent/comp/etw/impl"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/windowsdriver/procmon"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/cenkalti/backoff/v4"
	lru "github.com/hashicorp/golang-lru/v2"

	"golang.org/x/sys/windows"
)

// WindowsProbe defines a Windows probe
type WindowsProbe struct {
	Resolvers *resolvers.Resolvers

	// Constants and configuration
	opts         Opts
	config       *config.Config
	statsdClient statsd.ClientInterface

	// internals

	// note that these events are zeroed out and reused on every notification
	// what that means is that they're not thread safe; there needs to be one
	// event for each goroutine that's doing event processing.
	event             *model.Event
	ctx               context.Context
	cancelFnc         context.CancelFunc
	wg                sync.WaitGroup
	probe             *Probe
	fieldHandlers     *FieldHandlers
	pm                *procmon.WinProcmon
	onStart           chan *procmon.ProcessStartNotification
	onStop            chan *procmon.ProcessStopNotification
	onError           chan bool
	onETWNotification chan etwNotification

	// ETW component for FIM
	fileguid windows.GUID
	regguid  windows.GUID
	//etwcomp    etw.Component
	fimSession etw.Session
	fimwg      sync.WaitGroup

	// path caches
	filePathResolverLock sync.Mutex
	filePathResolver     map[fileObjectPointer]string
	regPathResolver      map[regObjectPointer]string

	// state tracking
	renamePreArgs renameState

	// stats
	stats stats
	// discarders
	discardedPaths     *lru.Cache[string, struct{}]
	discardedBasenames *lru.Cache[string, struct{}]
}

type renameState struct {
	fileObject uint64
	path       string
}

type etwNotification struct {
	arg any
	pid uint32
}

type stats struct {
	// driver notifications
	procStart uint64
	procStop  uint64

	// etw file notifications
	fileCreate    uint64
	fileCreateNew uint64
	fileCleanup   uint64
	fileClose     uint64
	fileFlush     uint64

	fileSetInformation     uint64
	fileSetDelete          uint64
	fileidRename           uint64
	fileidQueryInformation uint64
	fileidFSCTL            uint64
	fileidRename29         uint64

	// etw registry notifications
	regCreateKey   uint64
	regOpenKey     uint64
	regDeleteKey   uint64
	regFlushKey    uint64
	regCloseKey    uint64
	regSetValueKey uint64

	//filePathResolver status
	fileCreateSkippedDiscardedPaths     uint64
	fileCreateSkippedDiscardedBasenames uint64
}

/*
 * callback function for every etw notification, after it's been parsed.
 * pid is provided for testing purposes, to allow filtering on pid.  it is
 * not expected to be used at runtime
 */
type etwCallback func(n interface{}, pid uint32)

// Init initializes the probe
func (p *WindowsProbe) Init() error {

	if !p.opts.disableProcmon {
		pm, err := procmon.NewWinProcMon(p.onStart, p.onStop, p.onError, procmon.ProcmonDefaultReceiveSize, procmon.ProcmonDefaultNumBufs)
		if err != nil {
			return err
		}
		p.pm = pm
	}
	return p.initEtwFIM()
}

func (p *WindowsProbe) initEtwFIM() error {

	if !p.config.RuntimeSecurity.FIMEnabled {
		return nil
	}
	// log at Warning right now because it's not expected to be enabled
	log.Warnf("Enabling FIM processing")
	etwSessionName := "SystemProbeFIM_ETW"
	etwcomp, err := etwimpl.NewEtw()
	if err != nil {
		return err
	}
	p.fimSession, err = etwcomp.NewSession(etwSessionName)
	if err != nil {
		return err
	}

	// provider name="Microsoft-Windows-Kernel-File" guid="{edd08927-9cc4-4e65-b970-c2560fb5c289}"
	p.fileguid, err = windows.GUIDFromString("{edd08927-9cc4-4e65-b970-c2560fb5c289}")
	if err != nil {
		log.Errorf("Error converting guid %v", err)
		return err
	}

	//<provider name="Microsoft-Windows-Kernel-Registry" guid="{70eb4f03-c1de-4f73-a051-33d13d5413bd}"
	p.regguid, err = windows.GUIDFromString("{70eb4f03-c1de-4f73-a051-33d13d5413bd}")
	if err != nil {
		log.Errorf("Error converting guid %v", err)
		return err
	}

	pidsList := make([]uint32, 0)

	p.fimSession.ConfigureProvider(p.fileguid, func(cfg *etw.ProviderConfiguration) {
		cfg.TraceLevel = etw.TRACE_LEVEL_VERBOSE
		cfg.PIDs = pidsList

		// full manifest is here https://github.com/repnz/etw-providers-docs/blob/master/Manifests-Win10-17134/Microsoft-Windows-Kernel-File.xml
		/* the mask keywords available are
				<keywords>
					<keyword name="KERNEL_FILE_KEYWORD_FILENAME" message="$(string.keyword_KERNEL_FILE_KEYWORD_FILENAME)" mask="0x10"/>
					<keyword name="KERNEL_FILE_KEYWORD_FILEIO" message="$(string.keyword_KERNEL_FILE_KEYWORD_FILEIO)" mask="0x20"/>
					<keyword name="KERNEL_FILE_KEYWORD_OP_END" message="$(string.keyword_KERNEL_FILE_KEYWORD_OP_END)" mask="0x40"/>
					<keyword name="KERNEL_FILE_KEYWORD_CREATE" message="$(string.keyword_KERNEL_FILE_KEYWORD_CREATE)" mask="0x80"/>
					<keyword name="KERNEL_FILE_KEYWORD_READ" message="$(string.keyword_KERNEL_FILE_KEYWORD_READ)" mask="0x100"/>
					<keyword name="KERNEL_FILE_KEYWORD_WRITE" message="$(string.keyword_KERNEL_FILE_KEYWORD_WRITE)" mask="0x200"/>
					<keyword name="KERNEL_FILE_KEYWORD_DELETE_PATH" message="$(string.keyword_KERNEL_FILE_KEYWORD_DELETE_PATH)" mask="0x400"/>
					<keyword name="KERNEL_FILE_KEYWORD_RENAME_SETLINK_PATH" message="$(string.keyword_KERNEL_FILE_KEYWORD_RENAME_SETLINK_PATH)" mask="0x800"/>
					<keyword name="KERNEL_FILE_KEYWORD_CREATE_NEW_FILE" message="$(string.keyword_KERNEL_FILE_KEYWORD_CREATE_NEW_FILE)" mask="0x1000"/>
		    	</keywords>
		*/
		// try masking on create & create_new_file
		// given the current requirements, I think we can _probably_ just do create_new_file
		cfg.MatchAnyKeyword = 0x18A0
	})
	p.fimSession.ConfigureProvider(p.regguid, func(cfg *etw.ProviderConfiguration) {
		cfg.TraceLevel = etw.TRACE_LEVEL_VERBOSE
		cfg.PIDs = pidsList

		// full manifest is here https://github.com/repnz/etw-providers-docs/blob/master/Manifests-Win10-17134/Microsoft-Windows-Kernel-Registry.xml
		/* the mask keywords available are
				 <keywords>
					<keyword name="CloseKey" message="$(string.keyword_CloseKey)" mask="0x1"/>
					<keyword name="QuerySecurityKey" message="$(string.keyword_QuerySecurityKey)" mask="0x2"/>
					<keyword name="SetSecurityKey" message="$(string.keyword_SetSecurityKey)" mask="0x4"/>
					<keyword name="EnumerateValueKey" message="$(string.keyword_EnumerateValueKey)" mask="0x10"/>
					<keyword name="QueryMultipleValueKey" message="$(string.keyword_QueryMultipleValueKey)" mask="0x20"/>
					<keyword name="SetInformationKey" message="$(string.keyword_SetInformationKey)" mask="0x40"/>
					<keyword name="FlushKey" message="$(string.keyword_FlushKey)" mask="0x80"/>
					<keyword name="SetValueKey" message="$(string.keyword_SetValueKey)" mask="0x100"/>
					<keyword name="DeleteValueKey" message="$(string.keyword_DeleteValueKey)" mask="0x200"/>
					<keyword name="QueryValueKey" message="$(string.keyword_QueryValueKey)" mask="0x400"/>
					<keyword name="EnumerateKey" message="$(string.keyword_EnumerateKey)" mask="0x800"/>
					<keyword name="CreateKey" message="$(string.keyword_CreateKey)" mask="0x1000"/>
					<keyword name="OpenKey" message="$(string.keyword_OpenKey)" mask="0x2000"/>
					<keyword name="DeleteKey" message="$(string.keyword_DeleteKey)" mask="0x4000"/>
					<keyword name="QueryKey" message="$(string.keyword_QueryKey)" mask="0x8000"/>
		    	</keywords>
		*/
		// try masking on create & create_new_file
		// given the current requirements, I think we can _probably_ just do create_new_file
		cfg.MatchAnyKeyword = 0xF7E3
	})

	err = p.fimSession.EnableProvider(p.fileguid)
	if err != nil {
		log.Warnf("Error enabling provider %v", err)
		return err
	}
	err = p.fimSession.EnableProvider(p.regguid)
	if err != nil {
		log.Warnf("Error enabling provider %v", err)
		return err
	}
	return nil
}

// Setup the runtime security probe
func (p *WindowsProbe) Setup() error {
	return nil
}

// Stop the probe
func (p *WindowsProbe) Stop() {
	if p.fimSession != nil {
		_ = p.fimSession.StopTracing()
		p.fimwg.Wait()
	}
	if p.pm != nil {
		p.pm.Stop()
	}
}

func (p *WindowsProbe) setupEtw(ecb etwCallback) error {

	log.Info("Starting tracing...")
	err := p.fimSession.StartTracing(func(e *etw.DDEventRecord) {
		switch e.EventHeader.ProviderID {
		case etw.DDGUID(p.fileguid):
			switch e.EventHeader.EventDescriptor.ID {
			case idNameCreate:
				if ca, err := p.parseNameCreateArgs(e); err == nil {
					log.Tracef("Received nameCreate event %d %s\n", e.EventHeader.EventDescriptor.ID, ca)
					ecb(ca, e.EventHeader.ProcessID)
				}

			case idNameDelete:
				if ca, err := p.parseNameDeleteArgs(e); err == nil {
					log.Tracef("Received nameDelete event %d %s\n", e.EventHeader.EventDescriptor.ID, ca)
					ecb(ca, e.EventHeader.ProcessID)
				}

			case idCreate:
				if ca, err := p.parseCreateHandleArgs(e); err == nil {
					log.Tracef("Received idCreate event %d %s\n", e.EventHeader.EventDescriptor.ID, ca)
					ecb(ca, e.EventHeader.ProcessID)
				}
				p.stats.fileCreate++
			case idCreateNewFile:
				if ca, err := p.parseCreateNewFileArgs(e); err == nil {
					log.Tracef("Received idCreateNewFile event %d %s\n", e.EventHeader.EventDescriptor.ID, ca)
					ecb(ca, e.EventHeader.ProcessID)
				}
				p.stats.fileCreateNew++
			case idCleanup:
				if ca, err := p.parseCleanupArgs(e); err == nil {
					log.Tracef("Received cleanup event %d %s\n", e.EventHeader.EventDescriptor.ID, ca)
					ecb(ca, e.EventHeader.ProcessID)
				}
				p.stats.fileCleanup++
			case idClose:
				if ca, err := p.parseCloseArgs(e); err == nil {
					log.Tracef("Received Close event %d %s\n", e.EventHeader.EventDescriptor.ID, ca)
					ecb(ca, e.EventHeader.ProcessID)
					if e.EventHeader.EventDescriptor.ID == idClose {
						p.filePathResolverLock.Lock()
						delete(p.filePathResolver, ca.fileObject)
						p.filePathResolverLock.Unlock()
					}
				}
				p.stats.fileClose++
			case idFlush:
				if fa, err := p.parseFlushArgs(e); err == nil {
					ecb(fa, e.EventHeader.ProcessID)
				}
				p.stats.fileFlush++

			case idWrite:
				if wa, err := p.parseWriteArgs(e); err == nil {
					//fmt.Printf("Received Write event %d %s\n", e.EventHeader.EventDescriptor.ID, wa)
					log.Tracef("Received Write event %d %s\n", e.EventHeader.EventDescriptor.ID, wa)
					ecb(wa, e.EventHeader.ProcessID)
				}

			case idSetInformation:
				if si, err := p.parseInformationArgs(e); err == nil {
					log.Tracef("Received SetInformation event %d %s\n", e.EventHeader.EventDescriptor.ID, si)
					ecb(si, e.EventHeader.ProcessID)
				}
				p.stats.fileSetInformation++

			case idSetDelete:
				if sd, err := p.parseSetDeleteArgs(e); err == nil {
					log.Tracef("Received SetDelete event %d %s\n", e.EventHeader.EventDescriptor.ID, sd)
					ecb(sd, e.EventHeader.ProcessID)
				}
				p.stats.fileSetDelete++

			case idDeletePath:
				if dp, err := p.parseDeletePathArgs(e); err == nil {
					log.Tracef("Received DeletePath event %d %s\n", e.EventHeader.EventDescriptor.ID, dp)
					ecb(dp, e.EventHeader.ProcessID)
				}

			case idRename:
				if rn, err := p.parseRenameArgs(e); err == nil {
					log.Tracef("Received Rename event %d %s\n", e.EventHeader.EventDescriptor.ID, rn)
					ecb(rn, e.EventHeader.ProcessID)
				}
				p.stats.fileidRename++

			case idRenamePath:
				if rn, err := p.parseRenamePathArgs(e); err == nil {
					log.Tracef("Received RenamePath event %d %s\n", e.EventHeader.EventDescriptor.ID, rn)
					ecb(rn, e.EventHeader.ProcessID)
				}
			case idQueryInformation:
				p.stats.fileidQueryInformation++
			case idFSCTL:
				if fs, err := p.parseFsctlArgs(e); err == nil {
					log.Tracef("Received FSCTL event %d %s\n", e.EventHeader.EventDescriptor.ID, fs)
					ecb(fs, e.EventHeader.ProcessID)
				}
				p.stats.fileidFSCTL++

			case idRename29:
				if rn, err := p.parseRename29Args(e); err == nil {
					log.Tracef("Received Rename29 event %d %s\n", e.EventHeader.EventDescriptor.ID, rn)
					ecb(rn, e.EventHeader.ProcessID)
				}
				p.stats.fileidRename29++
			}

		case etw.DDGUID(p.regguid):
			switch e.EventHeader.EventDescriptor.ID {
			case idRegCreateKey:
				if cka, err := p.parseCreateRegistryKey(e); err == nil {
					log.Tracef("Got idRegCreateKey %s", cka)
					ecb(cka, e.EventHeader.ProcessID)
				}
				p.stats.regCreateKey++
			case idRegOpenKey:
				if cka, err := p.parseOpenRegistryKey(e); err == nil {
					log.Tracef("Got idRegOpenKey %s", cka)
					ecb(cka, e.EventHeader.ProcessID)
				}
				p.stats.regOpenKey++
			case idRegDeleteKey:
				if dka, err := p.parseDeleteRegistryKey(e); err == nil {
					log.Tracef("Got idRegDeleteKey %v", dka)
					ecb(dka, e.EventHeader.ProcessID)
				}
				p.stats.regDeleteKey++
			case idRegFlushKey:
				if dka, err := p.parseFlushKey(e); err == nil {
					log.Tracef("Got idRegFlushKey %v", dka)
				}
				p.stats.regFlushKey++
			case idRegCloseKey:
				if dka, err := p.parseCloseKeyArgs(e); err == nil {
					log.Tracef("Got idRegCloseKey %s", dka)
					delete(p.regPathResolver, dka.keyObject)
				}
				p.stats.regCloseKey++
			case idQuerySecurityKey:
				if dka, err := p.parseQuerySecurityKeyArgs(e); err == nil {
					log.Tracef("Got idQuerySecurityKey %v", dka.keyName)
				}
			case idSetSecurityKey:
				if dka, err := p.parseSetSecurityKeyArgs(e); err == nil {
					log.Tracef("Got idSetSecurityKey %v", dka.keyName)
				}
			case idRegSetValueKey:
				if svk, err := p.parseSetValueKey(e); err == nil {
					log.Tracef("Got idRegSetValueKey %s", svk)
					ecb(svk, e.EventHeader.ProcessID)

				}
				p.stats.regSetValueKey++

			}
		}
	})
	return err

}

// Start processing events
func (p *WindowsProbe) Start() error {

	log.Infof("Windows probe started")
	if p.fimSession != nil {
		// log at Warning right now because it's not expected to be enabled
		log.Warnf("Enabling FIM processing")
		p.fimwg.Add(1)

		go func() {
			defer p.fimwg.Done()
			err := p.setupEtw(func(n interface{}, pid uint32) {
				p.onETWNotification <- etwNotification{n, pid}
			})
			log.Infof("Done StartTracing %v", err)
		}()
	}
	if p.pm == nil {
		return nil
	}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		for {
			ev := p.zeroEvent()

			select {
			case <-p.ctx.Done():
				return

			case <-p.onError:
				// in this case, we got some sort of error that the underlying
				// subsystem can't recover from.  Need to initiate some sort of cleanup

			case start := <-p.onStart:
				if !p.handleProcessStart(ev, start) {
					continue
				}
			case stop := <-p.onStop:
				if !p.handleProcessStop(ev, stop) {
					continue
				}
			case notif := <-p.onETWNotification:
				if !p.handleETWNotification(ev, notif) {
					continue
				}
			}

			p.DispatchEvent(ev)
		}
	}()
	return p.pm.Start()
}

func (p *WindowsProbe) handleProcessStart(ev *model.Event, start *procmon.ProcessStartNotification) bool {
	pid := process.Pid(start.Pid)
	if pid == 0 {
		return false
	}
	p.stats.procStart++

	log.Debugf("Received start %v", start)

	// TODO
	// handle new fields
	// CreatingPRocessId
	// CreatingThreadId
	if start.RequiredSize != 0 {
		// in this case, the command line and/or the image file might not be filled in
		// depending upon how much space was needed.

		// potential actions
		// - just log/count the error and keep going
		// - restart underlying procmon with larger buffer size, at least if error keeps occurring
		log.Warnf("insufficient buffer size %v", start.RequiredSize)

	}

	pce, err := p.Resolvers.ProcessResolver.AddNewEntry(pid, uint32(start.PPid), start.ImageFile, start.CmdLine, start.OwnerSidString)
	if err != nil {
		log.Errorf("error in resolver %v", err)
		return false
	}
	ev.Type = uint32(model.ExecEventType)
	ev.Exec.Process = &pce.Process

	// use ProcessCacheEntry process context as process context
	ev.ProcessCacheEntry = pce
	ev.ProcessContext = &pce.ProcessContext

	p.Resolvers.ProcessResolver.DequeueExited()
	return true
}

func (p *WindowsProbe) handleProcessStop(ev *model.Event, stop *procmon.ProcessStopNotification) bool {
	pid := process.Pid(stop.Pid)
	if pid == 0 {
		// TODO this shouldn't happen
		return false
	}
	log.Debugf("Received stop %v", stop)
	p.stats.procStop++
	pce := p.Resolvers.ProcessResolver.GetEntry(pid)
	p.Resolvers.ProcessResolver.AddToExitedQueue(pid)

	ev.Type = uint32(model.ExitEventType)
	if pce == nil {
		log.Errorf("unable to resolve pid %d", pid)
		return false
	}
	ev.Exit.Process = &pce.Process
	// use ProcessCacheEntry process context as process context
	ev.ProcessCacheEntry = pce
	ev.ProcessContext = &pce.ProcessContext

	p.Resolvers.ProcessResolver.DequeueExited()
	return true
}

func (p *WindowsProbe) handleETWNotification(ev *model.Event, notif etwNotification) bool {
	// handle incoming events here
	// each event will come in as a different type
	// parse it with
	switch arg := notif.arg.(type) {
	case *createNewFileArgs:
		ev.Type = uint32(model.CreateNewFileEventType)
		ev.CreateNewFile = model.CreateNewFileEvent{
			File: model.FileEvent{
				FileObject:  uint64(arg.fileObject),
				PathnameStr: arg.fileName,
				BasenameStr: filepath.Base(arg.fileName),
			},
		}
	case *renameArgs:
		p.renamePreArgs = renameState{
			fileObject: uint64(arg.fileObject),
			path:       arg.fileName,
		}
	case *rename29Args:
		p.renamePreArgs = renameState{
			fileObject: uint64(arg.fileObject),
			path:       arg.fileName,
		}
	case *renamePath:
		ev.Type = uint32(model.FileRenameEventType)
		ev.RenameFile = model.RenameFileEvent{
			Old: model.FileEvent{
				FileObject:  p.renamePreArgs.fileObject,
				PathnameStr: p.renamePreArgs.path,
				BasenameStr: filepath.Base(p.renamePreArgs.path),
			},
			New: model.FileEvent{
				FileObject:  uint64(arg.fileObject),
				PathnameStr: arg.filePath,
				BasenameStr: filepath.Base(arg.filePath),
			},
		}

	case *createKeyArgs:
		ev.Type = uint32(model.CreateRegistryKeyEventType)
		ev.CreateRegistryKey = model.CreateRegistryKeyEvent{
			Registry: model.RegistryEvent{
				KeyPath: arg.computedFullPath,
				KeyName: filepath.Base(arg.computedFullPath),
			},
		}
	case *openKeyArgs:
		ev.Type = uint32(model.OpenRegistryKeyEventType)
		ev.OpenRegistryKey = model.OpenRegistryKeyEvent{
			Registry: model.RegistryEvent{
				KeyPath: arg.computedFullPath,
				KeyName: filepath.Base(arg.computedFullPath),
			},
		}
	case *deleteKeyArgs:
		ev.Type = uint32(model.DeleteRegistryKeyEventType)
		ev.DeleteRegistryKey = model.DeleteRegistryKeyEvent{
			Registry: model.RegistryEvent{
				KeyName: filepath.Base(arg.computedFullPath),
				KeyPath: arg.computedFullPath,
			},
		}
	case *setValueKeyArgs:
		ev.Type = uint32(model.SetRegistryKeyValueEventType)
		ev.SetRegistryKeyValue = model.SetRegistryKeyValueEvent{
			Registry: model.RegistryEvent{
				KeyName: filepath.Base(arg.computedFullPath),
				KeyPath: arg.computedFullPath,
			},
			ValueName: arg.valueName,
		}
	}

	if ev.Type != uint32(model.UnknownEventType) {
		errRes := p.setProcessContext(notif.pid, ev)
		if errRes != nil {
			log.Debugf("%v", errRes)
		}
	}
	return true
}

func (p *WindowsProbe) setProcessContext(pid uint32, event *model.Event) error {
	err := backoff.Retry(func() error {
		pce := p.Resolvers.ProcessResolver.GetEntry(pid)
		if pce == nil {
			return fmt.Errorf("Could not resolve process for Process: %v", pid)
		}
		event.ProcessCacheEntry = pce
		event.ProcessContext = &pce.ProcessContext
		return nil

	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(50*time.Millisecond), 5))
	return err
}

// DispatchEvent sends an event to the probe event handler
func (p *WindowsProbe) DispatchEvent(event *model.Event) {

	traceEvent("Dispatching event %s", func() ([]byte, model.EventType, error) {
		eventJSON, err := serializers.MarshalEvent(event, nil)
		return eventJSON, event.GetEventType(), err
	})

	// send event to wildcard handlers, like the CWS rule engine, first
	p.probe.sendEventToWildcardHandlers(event)

	// send event to specific event handlers, like the event monitor consumers, subsequently
	p.probe.sendEventToSpecificEventTypeHandlers(event)

}

// Snapshot runs the different snapshot functions of the resolvers that
// require to sync with the current state of the system
func (p *WindowsProbe) Snapshot() error {
	return p.Resolvers.Snapshot()
}

// Close the probe
func (p *WindowsProbe) Close() error {
	if p.pm != nil {
		p.pm.Stop()
	}

	p.cancelFnc()
	p.wg.Wait()
	return nil
}

// SendStats sends statistics about the probe to Datadog
func (p *WindowsProbe) SendStats() error {
	p.filePathResolverLock.Lock()
	fprLen := len(p.filePathResolver)
	p.filePathResolverLock.Unlock()

	// may need to lock here
	if err := p.statsdClient.Gauge(metrics.MetricWindowsProcessStart, float64(p.stats.procStart), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsProcessStop, float64(p.stats.procStop), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsFileCreate, float64(p.stats.fileCreate), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsFileCreateNew, float64(p.stats.fileCreateNew), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsFileCleanup, float64(p.stats.fileCleanup), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsFileClose, float64(p.stats.fileClose), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsFileFlush, float64(p.stats.fileFlush), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsFileSetInformation, float64(p.stats.fileSetInformation), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsFileSetDelete, float64(p.stats.fileSetDelete), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsFileIDRename, float64(p.stats.fileidRename), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsFileIDQueryInformation, float64(p.stats.fileidQueryInformation), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsFileIDFSCTL, float64(p.stats.fileidFSCTL), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsFileIDRename29, float64(p.stats.fileidRename29), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsFileCreateSkippedDiscardedPaths, float64(p.stats.fileCreateSkippedDiscardedPaths), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsFileCreateSkippedDiscardedBasenames, float64(p.stats.fileCreateSkippedDiscardedBasenames), nil, 1); err != nil {
		return err
	}

	if err := p.statsdClient.Gauge(metrics.MetricWindowsRegCreateKey, float64(p.stats.regCreateKey), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsRegOpenKey, float64(p.stats.regOpenKey), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsRegDeleteKey, float64(p.stats.regDeleteKey), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsRegFlushKey, float64(p.stats.regFlushKey), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsRegCloseKey, float64(p.stats.regCloseKey), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsRegSetValue, float64(p.stats.regSetValueKey), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsSizeOfFilePathResolver, float64(fprLen), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsSizeOfRegistryPathResolver, float64(len(p.regPathResolver)), nil, 1); err != nil {
		return err
	}
	return nil
}

// NewWindowsProbe instantiates a new runtime security agent probe
func NewWindowsProbe(probe *Probe, config *config.Config, opts Opts) (*WindowsProbe, error) {
	discardedPaths, err := lru.New[string, struct{}](1 << 10)
	if err != nil {
		return nil, err
	}

	discardedBasenames, err := lru.New[string, struct{}](1 << 10)
	if err != nil {
		return nil, err
	}

	ctx, cancelFnc := context.WithCancel(context.Background())

	p := &WindowsProbe{
		probe:             probe,
		config:            config,
		opts:              opts,
		statsdClient:      opts.StatsdClient,
		ctx:               ctx,
		cancelFnc:         cancelFnc,
		onStart:           make(chan *procmon.ProcessStartNotification),
		onStop:            make(chan *procmon.ProcessStopNotification),
		onError:           make(chan bool),
		onETWNotification: make(chan etwNotification),

		filePathResolver: make(map[fileObjectPointer]string, 0),
		regPathResolver:  make(map[regObjectPointer]string, 0),

		discardedPaths:     discardedPaths,
		discardedBasenames: discardedBasenames,
	}

	p.Resolvers, err = resolvers.NewResolvers(config, p.statsdClient, probe.scrubber)
	if err != nil {
		return nil, err
	}

	p.fieldHandlers = &FieldHandlers{config: config, resolvers: p.Resolvers}

	p.event = p.NewEvent()

	// be sure to zero the probe event before everything else
	p.zeroEvent()

	return p, nil
}

// ApplyRuleSet setup the probes for the provided set of rules and returns the policy report.
func (p *WindowsProbe) ApplyRuleSet(rs *rules.RuleSet) (*kfilters.ApplyRuleSetReport, error) {
	return kfilters.NewApplyRuleSetReport(p.config.Probe, rs)
}

// FlushDiscarders invalidates all the discarders
func (p *WindowsProbe) FlushDiscarders() error {
	p.discardedPaths.Purge()
	p.discardedBasenames.Purge()
	return nil
}

// OnNewDiscarder handles discarders
func (p *WindowsProbe) OnNewDiscarder(_ *rules.RuleSet, ev *model.Event, field eval.Field, evalType eval.EventType) {
	if evalType != "create" {
		return
	}

	if field == "create.file.path" {
		path := ev.CreateNewFile.File.PathnameStr
		seclog.Debugf("new discarder for `%s` -> `%v`", field, path)
		p.discardedPaths.Add(path, struct{}{})
	} else if field == "create.file.name" {
		basename := ev.CreateNewFile.File.BasenameStr
		seclog.Debugf("new discarder for `%s` -> `%v`", field, basename)
		p.discardedBasenames.Add(basename, struct{}{})
	}

	fileObject := fileObjectPointer(ev.CreateNewFile.File.FileObject)
	p.filePathResolverLock.Lock()
	defer p.filePathResolverLock.Unlock()
	delete(p.filePathResolver, fileObject)
}

// NewModel returns a new Model
func (p *WindowsProbe) NewModel() *model.Model {
	return NewWindowsModel(p)
}

// DumpDiscarders dump the discarders
func (p *WindowsProbe) DumpDiscarders() (string, error) {
	return "", errors.New("not supported")
}

// GetFieldHandlers returns the field handlers
func (p *WindowsProbe) GetFieldHandlers() model.FieldHandlers {
	return p.fieldHandlers
}

// DumpProcessCache dumps the process cache
func (p *WindowsProbe) DumpProcessCache(_ bool) (string, error) {
	return "", errors.New("not supported")
}

// NewEvent returns a new event
func (p *WindowsProbe) NewEvent() *model.Event {
	return NewWindowsEvent(p.fieldHandlers)
}

// HandleActions executes the actions of a triggered rule
func (p *WindowsProbe) HandleActions(_ *eval.Context, _ *rules.Rule) {}

// AddDiscarderPushedCallback add a callback to the list of func that have to be called when a discarder is pushed to kernel
func (p *WindowsProbe) AddDiscarderPushedCallback(_ DiscarderPushedCallback) {}

// GetEventTags returns the event tags
func (p *WindowsProbe) GetEventTags(_ string) []string {
	return nil
}

func (p *WindowsProbe) zeroEvent() *model.Event {
	p.event.Zero()
	p.event.FieldHandlers = p.fieldHandlers
	return p.event
}

// Origin returns origin
func (p *Probe) Origin() string {
	return ""
}

// NewProbe instantiates a new runtime security agent probe
func NewProbe(config *config.Config, opts Opts, _ optional.Option[workloadmeta.Component]) (*Probe, error) {
	opts.normalize()

	p := &Probe{
		Opts:         opts,
		Config:       config,
		StatsdClient: opts.StatsdClient,
		scrubber:     newProcScrubber(config.Probe.CustomSensitiveWords),
	}

	pp, err := NewWindowsProbe(p, config, opts)
	if err != nil {
		return nil, err
	}
	p.PlatformProbe = pp

	return p, nil
}
