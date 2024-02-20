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

	"github.com/DataDog/datadog-agent/comp/etw"
	etwimpl "github.com/DataDog/datadog-agent/comp/etw/impl"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	etwutil "github.com/DataDog/datadog-agent/pkg/util/winutil/etw"
	"github.com/DataDog/datadog-agent/pkg/windowsdriver/procmon"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/cenkalti/backoff"

	"golang.org/x/sys/windows"
)

var parseUnicodeString = etwutil.ParseUnicodeString

// WindowsProbe defines a Windows probe
type WindowsProbe struct {
	Resolvers *resolvers.Resolvers

	// Constants and configuration
	opts         Opts
	config       *config.Config
	statsdClient statsd.ClientInterface

	// internals
	event         *model.Event
	ctx           context.Context
	cancelFnc     context.CancelFunc
	wg            sync.WaitGroup
	probe         *Probe
	fieldHandlers *FieldHandlers
	pm            *procmon.WinProcmon
	onStart       chan *procmon.ProcessStartNotification
	onStop        chan *procmon.ProcessStopNotification
	onError       chan bool

	// ETW component for FIM
	fileguid windows.GUID
	regguid  windows.GUID
	//etwcomp    etw.Component
	fimSession etw.Session
	fimwg      sync.WaitGroup
}

/*
 * callback function for every etw notification, after it's been parsed.
 * pid is provided for testing purposes, to allow filtering on pid.  it is
 * not expected to be used at runtime
 */
type etwCallback func(n interface{}, pid uint32, eventType model.EventType)

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
		cfg.MatchAnyKeyword = 0x10A0
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
		//log.Infof("Received event %d for PID %d", e.EventHeader.EventDescriptor.ID, e.EventHeader.ProcessID)
		switch e.EventHeader.ProviderID {
		case etw.DDGUID(p.fileguid):
			switch e.EventHeader.EventDescriptor.ID {
			case idCreate:
				if ca, err := parseCreateHandleArgs(e); err == nil {
					log.Tracef("Received idCreate event %d %v\n", e.EventHeader.EventDescriptor.ID, ca.string())
					ecb(ca, e.EventHeader.ProcessID, model.UnknownEventType)
				}

			case idCreateNewFile:
				if ca, err := parseCreateNewFileArgs(e); err == nil {
					ecb(ca, e.EventHeader.ProcessID, model.CreateNewFileEventType)
				}
			case idCleanup:
				if ca, err := parseCleanupArgs(e); err == nil {
					ecb(ca, e.EventHeader.ProcessID, model.UnknownEventType)
				}

			case idClose:
				if ca, err := parseCloseArgs(e); err == nil {
					//fmt.Printf("Received Close event %d %v\n", e.EventHeader.EventDescriptor.ID, ca.string())
					ecb(ca, e.EventHeader.ProcessID, model.UnknownEventType)
					if e.EventHeader.EventDescriptor.ID == idClose {
						delete(filePathResolver, ca.fileObject)
					}
				}
			case idFlush:
				if fa, err := parseFlushArgs(e); err == nil {
					ecb(fa, e.EventHeader.ProcessID, model.UnknownEventType)
				}
			case idSetInformation:
				fallthrough
			case idSetDelete:
				fallthrough
			case idRename:
				fallthrough
			case idQueryInformation:
				fallthrough
			case idFSCTL:
				fallthrough
			case idRename29:
				if sia, err := parseInformationArgs(e); err == nil {
					log.Tracef("got id %v args %s", e.EventHeader.EventDescriptor.ID, sia.string())
				}
			}

		case etw.DDGUID(p.regguid):
			switch e.EventHeader.EventDescriptor.ID {
			case idRegCreateKey:
				if cka, err := parseCreateRegistryKey(e); err == nil {
					log.Tracef("Got idRegCreateKey %s", cka.string())
					ecb(cka, e.EventHeader.ProcessID, model.CreateRegistryKeyEventType)
				}
			case idRegOpenKey:
				if cka, err := parseCreateRegistryKey(e); err == nil {
					log.Debugf("Got idRegOpenKey %s", cka.string())
					ecb(cka, e.EventHeader.ProcessID, model.OpenRegistryKeyEventType)
				}

			case idRegDeleteKey:
				if dka, err := parseDeleteRegistryKey(e); err == nil {
					log.Tracef("Got idRegDeleteKey %v", dka.string())
					ecb(dka, e.EventHeader.ProcessID, model.DeleteRegistryKeyEventType)

				}
			case idRegFlushKey:
				if dka, err := parseDeleteRegistryKey(e); err == nil {
					log.Tracef("Got idRegFlushKey %v", dka.string())
				}
			case idRegCloseKey:
				if dka, err := parseDeleteRegistryKey(e); err == nil {
					log.Debugf("Got idRegCloseKey %s", dka.string())
					delete(regPathResolver, dka.keyObject)
				}
			case idQuerySecurityKey:
				if dka, err := parseDeleteRegistryKey(e); err == nil {
					log.Tracef("Got idQuerySecurityKey %v", dka.keyName)
				}
			case idSetSecurityKey:
				if dka, err := parseDeleteRegistryKey(e); err == nil {
					log.Tracef("Got idSetSecurityKey %v", dka.keyName)
				}
			case idRegSetValueKey:
				if svk, err := parseSetValueKey(e); err == nil {
					log.Tracef("Got idRegSetValueKey %s", svk.string())
					ecb(svk, e.EventHeader.ProcessID, model.SetRegistryKeyValueEventType)

				}

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
			err := p.setupEtw(func(n interface{}, pid uint32, eventType model.EventType) {
				// resolve process context
				ev := p.zeroEvent()

				// handle incoming events here
				// each event will come in as a different type
				// parse it with
				switch eventType {
				case model.CreateNewFileEventType:
					cnfa := n.(*createNewFileArgs)
					ev.Type = uint32(model.CreateNewFileEventType)
					ev.CreateNewFile = model.CreateNewFileEvent{
						File: model.FileEvent{
							PathnameStr: cnfa.fileName,
							BasenameStr: filepath.Base(cnfa.fileName),
						},
					}
				case model.CreateRegistryKeyEventType:
					cka := n.(*createKeyArgs)
					ev.Type = uint32(model.CreateRegistryKeyEventType)
					ev.CreateRegistryKey = model.CreateRegistryKeyEvent{
						Registry: model.RegistryEvent{
							KeyPath: cka.computedFullPath,
							KeyName: filepath.Base(cka.computedFullPath),
						},
					}
				case model.OpenRegistryKeyEventType:
					cka := n.(*createKeyArgs)
					ev.Type = uint32(model.OpenRegistryKeyEventType)
					ev.OpenRegistryKey = model.OpenRegistryKeyEvent{
						Registry: model.RegistryEvent{
							KeyPath: cka.computedFullPath,
							KeyName: filepath.Base(cka.computedFullPath),
						},
					}
				case model.DeleteRegistryKeyEventType:
					dka := n.(*deleteKeyArgs)
					ev.Type = uint32(model.DeleteRegistryKeyEventType)
					ev.DeleteRegistryKey = model.DeleteRegistryKeyEvent{
						Registry: model.RegistryEvent{
							KeyName: filepath.Base(dka.computedFullPath),
							KeyPath: dka.computedFullPath,
						},
					}
				case model.SetRegistryKeyValueEventType:
					svka := n.(*setValueKeyArgs)
					ev.Type = uint32(model.SetRegistryKeyValueEventType)
					ev.SetRegistryKeyValue = model.SetRegistryKeyValueEvent{
						Registry: model.RegistryEvent{
							KeyName:   filepath.Base(svka.computedFullPath),
							KeyPath:   svka.computedFullPath,
							ValueName: svka.valueName,
						},
					}
				}
				if ev.Type != uint32(model.UnknownEventType) {
					errRes := p.setProcessContext(pid, ev)
					if errRes != nil {
						log.Debugf("%v", errRes)
					}
					// Dispatch event
					p.DispatchEvent(ev)
				}

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
			var pce *model.ProcessCacheEntry
			var err error
			ev := p.zeroEvent()

			select {
			case <-p.ctx.Done():
				return

			case <-p.onError:
				// in this case, we got some sort of error that the underlying
				// subsystem can't recover from.  Need to initiate some sort of cleanup

			case start := <-p.onStart:
				pid := process.Pid(start.Pid)
				if pid == 0 {
					// TODO this shouldn't happen
					continue
				}

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

				pce, err = p.Resolvers.ProcessResolver.AddNewEntry(pid, uint32(start.PPid), start.ImageFile, start.CmdLine, start.OwnerSidString)
				if err != nil {
					log.Errorf("error in resolver %v", err)
					continue
				}
				ev.Type = uint32(model.ExecEventType)
				ev.Exec.Process = &pce.Process
			case stop := <-p.onStop:
				pid := process.Pid(stop.Pid)
				if pid == 0 {
					// TODO this shouldn't happen
					continue
				}
				log.Debugf("Received stop %v", stop)

				pce = p.Resolvers.ProcessResolver.GetEntry(pid)
				p.Resolvers.ProcessResolver.AddToExitedQueue(pid)

				ev.Type = uint32(model.ExitEventType)
				if pce == nil {
					log.Errorf("unable to resolve pid %d", pid)
					continue
				}
				ev.Exit.Process = &pce.Process
			}

			if pce == nil {
				continue
			}

			// use ProcessCacheEntry process context as process context
			ev.ProcessCacheEntry = pce
			ev.ProcessContext = &pce.ProcessContext

			p.Resolvers.ProcessResolver.DequeueExited()

			p.DispatchEvent(ev)

		}
	}()
	return p.pm.Start()
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
	return nil
}

// NewWindowsProbe instantiates a new runtime security agent probe
func NewWindowsProbe(probe *Probe, config *config.Config, opts Opts) (*WindowsProbe, error) {
	ctx, cancelFnc := context.WithCancel(context.Background())

	p := &WindowsProbe{
		probe:        probe,
		config:       config,
		opts:         opts,
		statsdClient: opts.StatsdClient,
		ctx:          ctx,
		cancelFnc:    cancelFnc,
		onStart:      make(chan *procmon.ProcessStartNotification),
		onStop:       make(chan *procmon.ProcessStopNotification),
		onError:      make(chan bool),
	}

	var err error
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
	return nil
}

// OnNewDiscarder handles discarders
func (p *WindowsProbe) OnNewDiscarder(_ *rules.RuleSet, _ *model.Event, _ eval.Field, _ eval.EventType) {
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

// NewProbe instantiates a new runtime security agent probe
func NewProbe(config *config.Config, opts Opts) (*Probe, error) {
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
