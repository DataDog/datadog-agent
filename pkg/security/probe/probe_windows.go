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
	"slices"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/cenkalti/backoff/v4"
	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
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
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/windowsdriver/procmon"

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
	fileguid  windows.GUID
	regguid   windows.GUID
	auditguid windows.GUID

	//etwcomp    etw.Component
	fimSession etw.Session
	fimwg      sync.WaitGroup

	// the audit session needs a separate ETW session because it's using
	// a well-known provider
	auditSession etw.Session

	// path caches
	filePathResolver *lru.Cache[fileObjectPointer, fileCache]
	regPathResolver  *lru.Cache[regObjectPointer, string]

	// state tracking
	renamePreArgs *lru.Cache[uint64, fileCache]

	// stats
	stats stats

	// discarders
	discardedPaths     *lru.Cache[string, struct{}]
	discardedUserPaths *lru.Cache[string, struct{}]
	discardedBasenames *lru.Cache[string, struct{}]

	discardedFileHandles *lru.Cache[fileObjectPointer, struct{}]

	// map of device path to volume name (i.e. c:)
	volumeMap map[string]string

	// actions
	processKiller *ProcessKiller

	// enabled probes
	isRenameEnabled           bool
	isWriteEnabled            bool
	isDeleteEnabled           bool
	isChangePermissionEnabled bool
	// channel handling.  Currently configurable, but should probably be set
	// to false with a configurable size value
	blockonchannelsend bool

	// approvers
	currentEventTypes []string
	approvers         map[eval.Field][]approver
	approverLock      sync.RWMutex
}

type approver interface {
	Approve(_ string) bool
}

type patternApprover struct {
	matcher *eval.PatternStringMatcher
}

// Approve the value
func (p *patternApprover) Approve(value string) bool {
	return p.matcher.Matches(value)
}

func newPatternApprover(pattern string) (*patternApprover, error) {
	var matcher eval.PatternStringMatcher
	if err := matcher.Compile(pattern, true); err != nil {
		return nil, err
	}
	return &patternApprover{
		matcher: &matcher,
	}, nil
}

// filecache currently only has a filename.  But this is going to expand really soon.  so go ahead
// and have the wrapper struct even though right now it doesn't add anything.
type fileCache struct {
	fileName     string
	userFileName string
}

type etwNotification struct {
	arg any
	pid uint32
}

type stats struct {
	// driver notifications
	procStart uint64
	procStop  uint64

	// file notifications
	fnLock            sync.Mutex
	fileNotifications map[uint16]uint64

	fpnLock                    sync.Mutex
	fileProcessedNotifications map[uint16]uint64

	// registry notifications
	rnLock                    sync.Mutex
	regNotifications          map[uint16]uint64
	rpnLock                   sync.Mutex
	regProcessedNotifications map[uint16]uint64

	//filePathResolver status
	fileCreateSkippedDiscardedPaths     uint64
	fileCreateSkippedDiscardedBasenames uint64

	fileNameCacheEvictions uint64
	registryCacheEvictions uint64

	// currently not used, reserved for future use
	etwChannelBlocked uint64

	// approver rejections
	createFileApproverRejects uint64

	totalEtwNotifications uint64
}

/*
 * callback function for every etw notification, after it's been parsed.
 * pid is provided for testing purposes, to allow filtering on pid.  it is
 * not expected to be used at runtime
 */
type etwCallback func(n interface{}, pid uint32)

// Init initializes the probe
func (p *WindowsProbe) Init() error {

	p.processKiller.Start(p.ctx, &p.wg)

	if !p.opts.disableProcmon {
		pm, err := procmon.NewWinProcMon(p.onStart, p.onStop, p.onError, procmon.ProcmonDefaultReceiveSize, procmon.ProcmonDefaultNumBufs)
		if err != nil {
			return err
		}
		p.pm = pm
	}
	return p.initEtwFIM()
}

// GetProfileManager returns the Profile Managers
func (p *WindowsProbe) GetProfileManager() interface{} {
	return nil
}

func (p *WindowsProbe) initEtwFIM() error {

	if !p.config.RuntimeSecurity.FIMEnabled {
		return nil
	}
	_ = p.initializeVolumeMap()
	// log at Warning right now because it's not expected to be enabled
	log.Warnf("Enabling FIM processing")
	etwSessionName := "SystemProbeFIM_ETW"
	auditSessionName := "EventLog-Security"
	etwcomp, err := etwimpl.NewEtw()
	if err != nil {
		return err
	}
	p.fimSession, err = etwcomp.NewSession(etwSessionName, nil)

	if err != nil {
		return err
	}
	if ls, err := winutil.IsCurrentProcessLocalSystem(); err == nil && ls {
		/* the well-known session requires being run as local system. It will initialize,
		   but no events will be sent.
		*/
		p.auditSession, err = etwcomp.NewWellKnownSession(auditSessionName, nil)
		if err != nil {
			return err
		}
		log.Info("Enabling the ETW auditing session")
	} else {
		if err != nil {
			log.Warnf("Unable to determine if we're running as local system %v", err)
		} else if !ls {
			log.Warnf("Not running as LOCAL_SYSTEM; audit events won't be captured")
		}
		log.Warnf("Not enabling the ETW auditing session")
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
	//  <provider name="Microsoft-Windows-Security-Auditing" guid="{54849625-5478-4994-a5ba-3e3b0328c30d}"
	p.auditguid, err = windows.GUIDFromString("{54849625-5478-4994-a5ba-3e3b0328c30d}")
	if err != nil {
		log.Errorf("Error converting guid %v", err)
		return err
	}

	return p.reconfigureProvider()
}

func (p *WindowsProbe) reconfigureProvider() error {
	if !p.config.RuntimeSecurity.FIMEnabled {
		return nil
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

		fileIDs := []uint16{
			idCreate,
			idCreateNewFile,
			idCleanup,
			idClose,
		}

		if p.isWriteEnabled {
			fileIDs = append(fileIDs, idWrite)
		}
		if p.isRenameEnabled {
			fileIDs = append(fileIDs, idRename, idRenamePath, idRename29)
		}
		if p.isDeleteEnabled {
			fileIDs = append(fileIDs, idSetDelete, idDeletePath)
		}
		if p.isChangePermissionEnabled {
			fileIDs = append(fileIDs, idObjectPermsChange)
		}

		cfg.EnabledIDs = fileIDs
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

	if p.auditSession != nil {
		p.auditSession.ConfigureProvider(p.auditguid, func(cfg *etw.ProviderConfiguration) {
			cfg.TraceLevel = etw.TRACE_LEVEL_VERBOSE
		})
	}

	if err := p.fimSession.EnableProvider(p.fileguid); err != nil {
		log.Warnf("Error enabling provider %v", err)
		return err
	}

	if err := p.fimSession.EnableProvider(p.regguid); err != nil {
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
	if p.fimSession != nil || p.auditSession != nil {
		if p.fimSession != nil {
			_ = p.fimSession.StopTracing()
		}
		if p.auditSession != nil {
			log.Info("Calling stoptracing on audit session")
			_ = p.auditSession.StopTracing()
		}
		p.fimwg.Wait()
	}
	if p.pm != nil {
		p.pm.Stop()
	}
}

func (p *WindowsProbe) approveFimBasename(value string) bool {
	fields := []string{"create.file.name", "rename.file.name", "delete.file.name", "write.file.name"}
	eventTypes := []string{"create", "rename", "delete", "write"}

	for i, field := range fields {
		eventType := eventTypes[i]
		if p.approve(field, eventType, value) {
			return true
		}
	}
	return false
}

// currently support only string base approver for now
func (p *WindowsProbe) approve(field eval.Field, eventType string, value string) bool {
	if !p.config.Probe.EnableApprovers {
		return true
	}

	p.approverLock.RLock()
	defer p.approverLock.RUnlock()

	approvers, exists := p.approvers[field]
	if !exists {
		// no approvers, so no filtering for this field, except if no rule for this event type
		return slices.Contains(p.currentEventTypes, eventType)
	}

	for _, approver := range approvers {
		if approver.Approve(value) {
			return true
		}
	}

	return false
}
func (p *WindowsProbe) auditEtw(ecb etwCallback) error {
	log.Info("Starting tracing...")
	err := p.auditSession.StartTracing(func(e *etw.DDEventRecord) {

		switch e.EventHeader.ProviderID {

		case etw.DDGUID(p.auditguid):
			switch e.EventHeader.EventDescriptor.ID {
			case idObjectPermsChange:
				if p.isChangePermissionEnabled {
					if pc, err := p.parseObjectPermsChange(e); err == nil {
						log.Infof("Received objectPermsChange event %d %s\n", e.EventHeader.EventDescriptor.ID, pc)
						ecb(pc, e.EventHeader.ProcessID)
					}
				}
			}
		}
	})
	return err
}

func (p *WindowsProbe) setupEtw(ecb etwCallback) error {

	log.Info("Starting tracing...")
	err := p.fimSession.StartTracing(func(e *etw.DDEventRecord) {
		p.stats.totalEtwNotifications++
		switch e.EventHeader.ProviderID {
		case etw.DDGUID(p.fileguid):

			p.stats.fnLock.Lock()
			p.stats.fileNotifications[e.EventHeader.EventDescriptor.ID]++
			p.stats.fnLock.Unlock()

			switch e.EventHeader.EventDescriptor.ID {
			case idNameCreate:
				if ca, err := p.parseNameCreateArgs(e); err == nil {
					log.Tracef("Received nameCreate event %d %s\n", e.EventHeader.EventDescriptor.ID, ca)

					p.stats.fpnLock.Lock()
					p.stats.fileProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
					p.stats.fpnLock.Unlock()

					ecb(ca, e.EventHeader.ProcessID)
				}

			case idNameDelete:
				if ca, err := p.parseNameDeleteArgs(e); err == nil {
					log.Tracef("Received nameDelete event %d %s\n", e.EventHeader.EventDescriptor.ID, ca)

					p.stats.fpnLock.Lock()
					p.stats.fileProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
					p.stats.fpnLock.Unlock()

					ecb(ca, e.EventHeader.ProcessID)
				}

			case idCreate:
				if ca, err := p.parseCreateHandleArgs(e); err == nil {
					log.Tracef("Received idCreate event %d %s\n", e.EventHeader.EventDescriptor.ID, ca)

					p.stats.fpnLock.Lock()
					p.stats.fileProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
					p.stats.fpnLock.Unlock()

					ecb(ca, e.EventHeader.ProcessID)
				}
			case idCreateNewFile:
				if ca, err := p.parseCreateNewFileArgs(e); err == nil {
					log.Tracef("Received idCreateNewFile event %d %s\n", e.EventHeader.EventDescriptor.ID, ca)

					p.stats.fpnLock.Lock()
					p.stats.fileProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
					p.stats.fpnLock.Unlock()

					ecb(ca, e.EventHeader.ProcessID)
				}
			case idCleanup:
				if ca, err := p.parseCleanupArgs(e); err == nil {
					log.Tracef("Received cleanup event %d %s\n", e.EventHeader.EventDescriptor.ID, ca)

					p.stats.fpnLock.Lock()
					p.stats.fileProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
					p.stats.fpnLock.Unlock()

					ecb(ca, e.EventHeader.ProcessID)
				}
			case idClose:
				if ca, err := p.parseCloseArgs(e); err == nil {
					log.Tracef("Received Close event %d %s\n", e.EventHeader.EventDescriptor.ID, ca)

					p.stats.fpnLock.Lock()
					p.stats.fileProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
					p.stats.fpnLock.Unlock()

					ecb(ca, e.EventHeader.ProcessID)
					// lru is thread safe, has its own locking
					p.discardedFileHandles.Remove(ca.fileObject)
					p.filePathResolver.Remove(ca.fileObject)
				}
			case idFlush:
				if fa, err := p.parseFlushArgs(e); err == nil {

					p.stats.fpnLock.Lock()
					p.stats.fileProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
					p.stats.fpnLock.Unlock()

					ecb(fa, e.EventHeader.ProcessID)
				}

			case idWrite:
				if p.isWriteEnabled {
					if wa, err := p.parseWriteArgs(e); err == nil {
						//fmt.Printf("Received Write event %d %s\n", e.EventHeader.EventDescriptor.ID, wa)
						log.Tracef("Received Write event %d %s\n", e.EventHeader.EventDescriptor.ID, wa)
						ecb(wa, e.EventHeader.ProcessID)
						p.stats.fpnLock.Lock()
						p.stats.fileProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
						p.stats.fpnLock.Unlock()
					}
				}

			case idSetInformation:
				if si, err := p.parseInformationArgs(e); err == nil {
					log.Tracef("Received SetInformation event %d %s\n", e.EventHeader.EventDescriptor.ID, si)
					ecb(si, e.EventHeader.ProcessID)
					p.stats.fpnLock.Lock()
					p.stats.fileProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
					p.stats.fpnLock.Unlock()
				}

			case idSetDelete:
				if p.isDeleteEnabled {
					if sd, err := p.parseSetDeleteArgs(e); err == nil {
						log.Tracef("Received SetDelete event %d %s\n", e.EventHeader.EventDescriptor.ID, sd)
						ecb(sd, e.EventHeader.ProcessID)

						p.stats.fpnLock.Lock()
						p.stats.fileProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
						p.stats.fpnLock.Unlock()
					}
				}
			case idDeletePath:
				if p.isDeleteEnabled {
					if dp, err := p.parseDeletePathArgs(e); err == nil {
						log.Tracef("Received DeletePath event %d %s\n", e.EventHeader.EventDescriptor.ID, dp)

						p.stats.fpnLock.Lock()
						p.stats.fileProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
						p.stats.fpnLock.Unlock()

						ecb(dp, e.EventHeader.ProcessID)
					}
				}

			case idRename:
				if p.isRenameEnabled {
					if rn, err := p.parseRenameArgs(e); err == nil {
						log.Tracef("Received Rename event %d %s\n", e.EventHeader.EventDescriptor.ID, rn)
						ecb(rn, e.EventHeader.ProcessID)

						p.stats.fpnLock.Lock()
						p.stats.fileProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
						p.stats.fpnLock.Unlock()
					}
				}
			case idRenamePath:
				if p.isRenameEnabled {
					if rn, err := p.parseRenamePathArgs(e); err == nil {
						log.Tracef("Received RenamePath event %d %s\n", e.EventHeader.EventDescriptor.ID, rn)
						ecb(rn, e.EventHeader.ProcessID)
						p.stats.fpnLock.Lock()
						p.stats.fileProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
						p.stats.fpnLock.Unlock()
					}
				}
			case idFSCTL:
				if fs, err := p.parseFsctlArgs(e); err == nil {
					log.Tracef("Received FSCTL event %d %s\n", e.EventHeader.EventDescriptor.ID, fs)
					ecb(fs, e.EventHeader.ProcessID)
					p.stats.fpnLock.Lock()
					p.stats.fileProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
					p.stats.fpnLock.Unlock()
				}

			case idRename29:
				if p.isRenameEnabled {
					if rn, err := p.parseRename29Args(e); err == nil {
						log.Tracef("Received Rename29 event %d %s\n", e.EventHeader.EventDescriptor.ID, rn)
						ecb(rn, e.EventHeader.ProcessID)
					}
					p.stats.fpnLock.Lock()
					p.stats.fileProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
					p.stats.fpnLock.Unlock()
				}
			}

		case etw.DDGUID(p.regguid):
			p.stats.rnLock.Lock()
			p.stats.regNotifications[e.EventHeader.EventDescriptor.ID]++
			p.stats.rnLock.Unlock()
			switch e.EventHeader.EventDescriptor.ID {
			case idRegCreateKey:
				if cka, err := p.parseCreateRegistryKey(e); err == nil {
					log.Tracef("Got idRegCreateKey %s", cka)
					ecb(cka, e.EventHeader.ProcessID)
					p.stats.rpnLock.Lock()
					p.stats.regProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
					p.stats.rpnLock.Unlock()

				}
			case idRegOpenKey:
				if cka, err := p.parseOpenRegistryKey(e); err == nil {
					log.Tracef("Got idRegOpenKey %s", cka)
					ecb(cka, e.EventHeader.ProcessID)
					p.stats.rpnLock.Lock()
					p.stats.regProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
					p.stats.rpnLock.Unlock()
				}
			case idRegDeleteKey:
				if dka, err := p.parseDeleteRegistryKey(e); err == nil {
					log.Tracef("Got idRegDeleteKey %v", dka)
					ecb(dka, e.EventHeader.ProcessID)
					p.stats.rpnLock.Lock()
					p.stats.regProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
					p.stats.rpnLock.Unlock()

				}
			case idRegFlushKey:
				if dka, err := p.parseFlushKey(e); err == nil {
					log.Tracef("Got idRegFlushKey %v", dka)
					p.stats.rpnLock.Lock()
					p.stats.regProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
					p.stats.rpnLock.Unlock()
				}
			case idRegCloseKey:
				if dka, err := p.parseCloseKeyArgs(e); err == nil {
					log.Tracef("Got idRegCloseKey %s", dka)
					p.regPathResolver.Remove(dka.keyObject)
					p.stats.rpnLock.Lock()
					p.stats.regProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
					p.stats.rpnLock.Unlock()

				}
			case idQuerySecurityKey:
				if dka, err := p.parseQuerySecurityKeyArgs(e); err == nil {
					log.Tracef("Got idQuerySecurityKey %v", dka.keyName)
					p.stats.rpnLock.Lock()
					p.stats.regProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
					p.stats.rpnLock.Unlock()
				}
			case idSetSecurityKey:
				if dka, err := p.parseSetSecurityKeyArgs(e); err == nil {
					log.Tracef("Got idSetSecurityKey %v", dka.keyName)
					p.stats.rpnLock.Lock()
					p.stats.regProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
					p.stats.rpnLock.Unlock()
				}
			case idRegSetValueKey:
				if svk, err := p.parseSetValueKey(e); err == nil {
					log.Tracef("Got idRegSetValueKey %s", svk)
					ecb(svk, e.EventHeader.ProcessID)
					p.stats.rpnLock.Lock()
					p.stats.regProcessedNotifications[e.EventHeader.EventDescriptor.ID]++
					p.stats.rpnLock.Unlock()
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
			err := p.setupEtw(func(n interface{}, pid uint32) {
				if p.blockonchannelsend {
					p.onETWNotification <- etwNotification{n, pid}
				} else {
					select {
					case p.onETWNotification <- etwNotification{n, pid}:
					default:
						p.stats.etwChannelBlocked++
					}
				}
			})
			log.Infof("Done StartTracing %v", err)
		}()
	}
	if p.auditSession != nil {
		log.Warnf("Enabling Audit processing")
		p.fimwg.Add(1)
		go func() {
			defer p.fimwg.Done()
			err := p.auditEtw(func(n interface{}, pid uint32) {
				p.onETWNotification <- etwNotification{n, pid}
			})
			log.Infof("Done AuditTracing %v", err)
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
				log.Errorf("error in underlying procmon")
				continue
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
				p.handleETWNotification(ev, notif)
			}

			p.DispatchEvent(ev)

			// flush pending kill actions
			p.processKiller.FlushPendingReports()
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

	pce, err := p.Resolvers.ProcessResolver.AddNewEntry(pid, uint32(start.PPid), start.ImageFile, start.EnvBlock, start.CmdLine, start.OwnerSidString)
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
	pce.ExitTime = time.Now()
	ev.Exit.Process = &pce.Process
	// use ProcessCacheEntry process context as process context
	ev.ProcessCacheEntry = pce
	ev.ProcessContext = &pce.ProcessContext

	// update kill action reports
	p.processKiller.HandleProcessExited(ev)

	p.Resolvers.ProcessResolver.DequeueExited()
	return true
}

func (p *WindowsProbe) handleETWNotification(ev *model.Event, notif etwNotification) {
	// handle incoming events here
	// each event will come in as a different type
	// parse it with
	switch arg := notif.arg.(type) {
	case *createNewFileArgs:
		ev.Type = uint32(model.CreateNewFileEventType)
		ev.CreateNewFile = model.CreateNewFileEvent{
			File: model.FimFileEvent{
				FileObject:      uint64(arg.fileObject),
				PathnameStr:     arg.fileName,
				UserPathnameStr: arg.userFileName,
				BasenameStr:     filepath.Base(arg.fileName),
			},
		}
	case *renameArgs:
		fc := fileCache{
			fileName:     arg.fileName,
			userFileName: arg.userFileName,
		}
		p.renamePreArgs.Add(uint64(arg.fileObject), fc)
	case *rename29Args:
		fc := fileCache{
			fileName:     arg.fileName,
			userFileName: arg.userFileName,
		}
		p.renamePreArgs.Add(uint64(arg.fileObject), fc)
	case *renamePath:
		fileCache, found := p.renamePreArgs.Get(uint64(arg.fileObject))
		if !found {
			log.Debugf("unable to find renamePreArgs for %d", uint64(arg.fileObject))
			return
		}
		ev.Type = uint32(model.FileRenameEventType)
		ev.RenameFile = model.RenameFileEvent{
			Old: model.FimFileEvent{
				FileObject:      uint64(arg.fileObject),
				PathnameStr:     fileCache.fileName,
				UserPathnameStr: fileCache.userFileName,
				BasenameStr:     filepath.Base(fileCache.fileName),
			},
			New: model.FimFileEvent{
				FileObject:      uint64(arg.fileObject),
				PathnameStr:     arg.filePath,
				UserPathnameStr: arg.userFilePath,
				BasenameStr:     filepath.Base(arg.filePath),
			},
		}
		p.renamePreArgs.Remove(uint64(arg.fileObject))
	case *setDeleteArgs:
		ev.Type = uint32(model.DeleteFileEventType)
		ev.DeleteFile = model.DeleteFileEvent{
			File: model.FimFileEvent{
				FileObject:      uint64(arg.fileObject),
				PathnameStr:     arg.fileName,
				UserPathnameStr: arg.userFileName,
				BasenameStr:     filepath.Base(arg.fileName),
			},
		}
	case *writeArgs:
		ev.Type = uint32(model.WriteFileEventType)
		ev.WriteFile = model.WriteFileEvent{
			File: model.FimFileEvent{
				FileObject:      uint64(arg.fileObject),
				PathnameStr:     arg.fileName,
				UserPathnameStr: arg.userFileName,
				BasenameStr:     filepath.Base(arg.fileName),
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
	case *objectPermsChange:
		ev.Type = uint32(model.ChangePermissionEventType)
		ev.ChangePermission = model.ChangePermissionEvent{
			UserName:   arg.subjectUserName,
			UserDomain: arg.subjectDomainName,
			ObjectName: arg.objectName,
			ObjectType: arg.objectType,
			OldSd:      arg.oldSd,
			NewSd:      arg.newSd,
		}
	}

	if ev.Type != uint32(model.UnknownEventType) {
		errRes := p.setProcessContext(notif.pid, ev)
		if errRes != nil {
			log.Debugf("%v", errRes)
		}
	}
}

func (p *WindowsProbe) setProcessContext(pid uint32, event *model.Event) error {
	event.PIDContext.Pid = pid
	err := backoff.Retry(func() error {
		entry, isResolved := p.fieldHandlers.ResolveProcessCacheEntry(event)
		event.ProcessCacheEntry = entry
		// use ProcessCacheEntry process context as process context
		event.ProcessContext = &event.ProcessCacheEntry.ProcessContext
		if !isResolved {
			return fmt.Errorf("could not resolve process for Process: %v", pid)
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(50*time.Millisecond), 5))

	if event.ProcessCacheEntry == nil {
		panic("should always return a process cache entry")
	}

	if event.ProcessContext == nil {
		panic("should always return a process context")
	}

	return err
}

// DispatchEvent sends an event to the probe event handler
func (p *WindowsProbe) DispatchEvent(event *model.Event) {
	traceEvent("Dispatching event %s", func() ([]byte, model.EventType, error) {
		eventJSON, err := serializers.MarshalEvent(event, nil)
		return eventJSON, event.GetEventType(), err
	})

	// send event to wildcard handlers, like the CWS rule engine, first
	p.probe.sendEventToHandlers(event)

	// send event to specific event handlers, like the event monitor consumers, subsequently
	p.probe.sendEventToConsumers(event)
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
	fprLen := p.filePathResolver.Len()

	// may need to lock here
	if err := p.statsdClient.Gauge(metrics.MetricWindowsProcessStart, float64(p.stats.procStart), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsProcessStop, float64(p.stats.procStop), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsFileCreateSkippedDiscardedPaths, float64(p.stats.fileCreateSkippedDiscardedPaths), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsFileCreateSkippedDiscardedBasenames, float64(p.stats.fileCreateSkippedDiscardedBasenames), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsFilePathEvictions, float64(p.stats.fileNameCacheEvictions), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsRegPathEvictions, float64(p.stats.registryCacheEvictions), nil, 1); err != nil {
		return err
	}

	if err := p.statsdClient.Gauge(metrics.MetricWindowsSizeOfFilePathResolver, float64(fprLen), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsSizeOfRegistryPathResolver, float64(p.regPathResolver.Len()), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsETWChannelBlockedCount, float64(p.stats.etwChannelBlocked), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsETWTotalNotifications, float64(p.stats.totalEtwNotifications), nil, 1); err != nil {
		return err
	}
	if err := p.statsdClient.Gauge(metrics.MetricWindowsApproverRejects, float64(p.stats.createFileApproverRejects), nil, 1); err != nil {
		return err
	}
	if p.fimSession == nil {
		return nil
	}

	// all stats below this line only valid if the full ETW session is enabled

	if etwstats, err := p.fimSession.GetSessionStatistics(); err == nil {
		if err := p.statsdClient.Gauge(metrics.MetricWindowsETWNumberOfBuffers, float64(etwstats.NumberOfBuffers), nil, 1); err != nil {
			return err
		}
		if err := p.statsdClient.Gauge(metrics.MetricWindowsETWFreeBuffers, float64(etwstats.FreeBuffers), nil, 1); err != nil {
			return err
		}
		if err := p.statsdClient.Gauge(metrics.MetricWindowsETWEventsLost, float64(etwstats.EventsLost), nil, 1); err != nil {
			return err
		}
		if err := p.statsdClient.Gauge(metrics.MetricWindowsETWBuffersWritten, float64(etwstats.BuffersWritten), nil, 1); err != nil {
			return err
		}
		if err := p.statsdClient.Gauge(metrics.MetricWindowsETWLogBuffersLost, float64(etwstats.LogBuffersLost), nil, 1); err != nil {
			return err
		}
		if err := p.statsdClient.Gauge(metrics.MetricWindowsETWRealTimeBuffersLost, float64(etwstats.RealTimeBuffersLost), nil, 1); err != nil {
			return err
		}
	}
	p.stats.fnLock.Lock()
	err := p.sendMapStats(&p.stats.fileNotifications, metrics.MetricWindowsFileNotifications)
	p.stats.fnLock.Unlock()
	if err != nil {
		return err
	}
	p.stats.fpnLock.Lock()
	err = p.sendMapStats(&p.stats.fileProcessedNotifications, metrics.MetricWindowsFileNotificationsProcessed)
	p.stats.fpnLock.Unlock()
	if err != nil {
		return err
	}
	p.stats.rnLock.Lock()
	err = p.sendMapStats(&p.stats.regNotifications, metrics.MetricWindowsRegistryNotifications)
	p.stats.rnLock.Unlock()
	if err != nil {
		return err
	}
	p.stats.rpnLock.Lock()
	err = p.sendMapStats(&p.stats.regProcessedNotifications, metrics.MetricWindowsRegistryNotificationsProcessed)
	p.stats.rpnLock.Unlock()
	if err != nil {
		return err
	}

	p.processKiller.SendStats(p.statsdClient)

	return nil
}

func (p *WindowsProbe) sendMapStats(m *map[uint16]uint64, metric string) error {
	for k, v := range *m {
		if err := p.statsdClient.Gauge(metric, float64(v), []string{fmt.Sprintf("event_id:%d", k)}, 1); err != nil {
			return err
		}
	}
	return nil
}

func initializeWindowsProbe(config *config.Config, opts Opts) (*WindowsProbe, error) {
	discardedPaths, err := lru.New[string, struct{}](1 << 10)
	if err != nil {
		return nil, err
	}

	discardedUserPaths, err := lru.New[string, struct{}](1 << 10)
	if err != nil {
		return nil, err
	}

	discardedBasenames, err := lru.New[string, struct{}](1 << 10)
	if err != nil {
		return nil, err
	}

	fc, err := lru.New[fileObjectPointer, fileCache](config.RuntimeSecurity.WindowsFilenameCacheSize)
	if err != nil {
		return nil, err
	}
	rc, err := lru.New[regObjectPointer, string](config.RuntimeSecurity.WindowsRegistryCacheSize)
	if err != nil {
		return nil, err
	}
	dfh, err := lru.New[fileObjectPointer, struct{}](config.RuntimeSecurity.WindowsFilenameCacheSize)
	if err != nil {
		return nil, err
	}

	rnc, err := lru.New[uint64, fileCache](5)
	if err != nil {
		return nil, err
	}

	bocs := config.RuntimeSecurity.WindowsProbeBlockOnChannelSend

	etwNotificationSize := config.RuntimeSecurity.ETWEventsChannelSize
	log.Infof("Setting ETW channel size to %d", etwNotificationSize)

	processKiller, err := NewProcessKiller(config)
	if err != nil {
		return nil, err
	}

	ctx, cancelFnc := context.WithCancel(context.Background())

	p := &WindowsProbe{
		config:            config,
		opts:              opts,
		statsdClient:      opts.StatsdClient,
		ctx:               ctx,
		cancelFnc:         cancelFnc,
		onStart:           make(chan *procmon.ProcessStartNotification),
		onStop:            make(chan *procmon.ProcessStopNotification),
		onError:           make(chan bool),
		onETWNotification: make(chan etwNotification, etwNotificationSize),

		filePathResolver: fc,
		regPathResolver:  rc,

		renamePreArgs: rnc,

		discardedPaths:     discardedPaths,
		discardedUserPaths: discardedUserPaths,
		discardedBasenames: discardedBasenames,

		discardedFileHandles: dfh,

		approvers: make(map[eval.Field][]approver),

		volumeMap: make(map[string]string),

		processKiller: processKiller,

		blockonchannelsend: bocs,

		stats: stats{
			fileNotifications:          make(map[uint16]uint64),
			fileProcessedNotifications: make(map[uint16]uint64),
			regNotifications:           make(map[uint16]uint64),
			regProcessedNotifications:  make(map[uint16]uint64),
		},
	}
	return p, nil
}

// NewWindowsProbe instantiates a new runtime security agent probe
func NewWindowsProbe(probe *Probe, config *config.Config, opts Opts, telemetry telemetry.Component) (*WindowsProbe, error) {
	p, err := initializeWindowsProbe(config, opts)
	if err != nil {
		return nil, err
	}
	p.probe = probe
	p.Resolvers, err = resolvers.NewResolvers(config, p.statsdClient, probe.scrubber, telemetry)
	if err != nil {
		return nil, err
	}

	hostname, err := utils.GetHostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}

	p.fieldHandlers = &FieldHandlers{config: config, resolvers: p.Resolvers, hostname: hostname}

	p.event = p.NewEvent()

	// be sure to zero the probe event before everything else
	p.zeroEvent()

	return p, nil
}

// ApplyRuleSet setup the probes for the provided set of rules and returns the policy report.
func (p *WindowsProbe) ApplyRuleSet(rs *rules.RuleSet) (*kfilters.ApplyRuleSetReport, error) {
	p.isWriteEnabled = false
	p.isRenameEnabled = false
	p.isDeleteEnabled = false
	p.isChangePermissionEnabled = false
	p.currentEventTypes = rs.GetEventTypes()

	for _, eventType := range p.currentEventTypes {
		switch eventType {
		case model.FileRenameEventType.String():
			p.isRenameEnabled = true
		case model.WriteFileEventType.String():
			p.isWriteEnabled = true
		case model.DeleteFileEventType.String():
			p.isDeleteEnabled = true
		case model.ChangePermissionEventType.String():
			p.isChangePermissionEnabled = true
		}
	}

	ars, err := kfilters.NewApplyRuleSetReport(p.config.Probe, rs)
	if err != nil {
		return nil, err
	}

	// remove old approvers
	p.approverLock.Lock()
	defer p.approverLock.Unlock()

	clear(p.approvers)

	for eventType, report := range ars.Policies {
		if err := p.setApprovers(eventType, report.Approvers); err != nil {
			return nil, err
		}
	}

	if err := p.reconfigureProvider(); err != nil {
		return nil, err
	}

	return ars, nil
}

// OnNewRuleSetLoaded resets statistics and states once a new rule set is loaded
func (p *WindowsProbe) OnNewRuleSetLoaded(rs *rules.RuleSet) {
	p.processKiller.Reset(rs)
}

// FlushDiscarders invalidates all the discarders
func (p *WindowsProbe) FlushDiscarders() error {
	p.discardedPaths.Purge()
	p.discardedUserPaths.Purge()
	p.discardedBasenames.Purge()
	return nil
}

// OnNewDiscarder handles discarders
func (p *WindowsProbe) OnNewDiscarder(_ *rules.RuleSet, ev *model.Event, field eval.Field, evalType eval.EventType) {
	if !p.config.Probe.EnableDiscarders {
		return
	}

	if evalType != "create" {
		return
	}

	if field == "create.file.device_path" {
		path := ev.CreateNewFile.File.PathnameStr
		seclog.Debugf("new discarder for `%s` -> `%v`", field, path)
		p.discardedPaths.Add(path, struct{}{})
	} else if field == "create.file.path" {
		path := ev.CreateNewFile.File.UserPathnameStr
		if path == "" {
			return
		}
		seclog.Debugf("new discarder for `%s` -> `%v`", field, path)
		p.discardedUserPaths.Add(path, struct{}{})
	} else if field == "create.file.name" {
		basename := ev.CreateNewFile.File.BasenameStr
		seclog.Debugf("new discarder for `%s` -> `%v`", field, basename)
		p.discardedBasenames.Add(basename, struct{}{})
	}

	fileObject := fileObjectPointer(ev.CreateNewFile.File.FileObject)
	p.filePathResolver.Remove(fileObject)
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
func (p *WindowsProbe) HandleActions(ctx *eval.Context, rule *rules.Rule) {
	ev := ctx.Event.(*model.Event)

	for _, action := range rule.Actions {
		if !action.IsAccepted(ctx) {
			continue
		}

		switch {
		case action.Def.Kill != nil:
			// do not handle kill action on event with error
			if ev.Error != nil {
				return
			}

			if p.processKiller.KillAndReport(action.Def.Kill, rule, ev, func(pid uint32, sig uint32) error {
				return p.processKiller.KillFromUserspace(pid, sig, ev)
			}) {
				p.probe.onRuleActionPerformed(rule, action.Def)
			}
		}
	}
}

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

// EnableEnforcement sets the enforcement mode
func (p *WindowsProbe) EnableEnforcement(state bool) {
	p.processKiller.SetState(state)
}

// NewProbe instantiates a new runtime security agent probe
func NewProbe(config *config.Config, opts Opts, telemetry telemetry.Component) (*Probe, error) {
	opts.normalize()

	p := newProbe(config, opts)

	pp, err := NewWindowsProbe(p, config, opts, telemetry)
	if err != nil {
		return nil, err
	}
	p.PlatformProbe = pp

	return p, nil
}

// setApprovers applies approvers and removes the unused ones
func (p *WindowsProbe) setApprovers(_ eval.EventType, approvers rules.Approvers) error {
	for name, els := range approvers {
		for _, el := range els {
			if el.Type == eval.ScalarValueType || el.Type == eval.PatternValueType {
				value, ok := el.Value.(string)
				if !ok {
					return errors.New("invalid pattern type")
				}

				ap, err := newPatternApprover(value)
				if err != nil {
					return err
				}
				l := p.approvers[name]
				p.approvers[name] = append(l, ap)
			}
		}
	}

	return nil
}
