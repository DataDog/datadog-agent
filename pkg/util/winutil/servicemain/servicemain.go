// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

// Package servicemain provides Windows Service application helpers
package servicemain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/messagestrings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

// Service defines the interface that applications should implement to run as Windows Services
type Service interface {
	// Name() returns the string to be used as the source for event log records.
	Name() string

	// Init() implements application initialization and is run when the service status is SERVICE_START_PENDING.
	// The service status is set to SERVICE_RUNNING when Init() returns successfully.
	// See ErrCleanStopAfterInit if you need to exit without calling Service.Run() or throwing an error.
	//
	// This function will block service tools like PowerShell's `Start-Service` until it returns.
	Init() error

	// Run() implements all application logic and is run when the service status is SERVICE_RUNNING.
	//
	// The provided context is cancellable. Run() must monitor ctx.Done() and return as soon as possible
	// when it is set. There is no standard time limit, it is up to the program performing the service stop.
	// The Services.msc snap-in has a 125 second limit, if the operating system is rebooting there is a 20 second limit.
	// https://learn.microsoft.com/en-us/windows/win32/services/service-control-handler-function
	//
	// The service will exit when Run() returns. Run() must return for the service status to be be updated to SERVICE_STOPPED.
	// If the process exits without setting SERVICE_STOPPED, Service Control Manager (SCM) will treat
	// this as an unexpected exit and enter failure/recovery, regardless of the process exit code.
	// https://learn.microsoft.com/en-us/windows/win32/api/winsvc/ns-winsvc-service_failure_actionsa
	Run(ctx context.Context) error
}

// ErrCleanStopAfterInit should be returned from Service.Init() to report SERVICE_RUNNING and then exit without error after
// a delay. See runTimeExitGate for more information on why the delay is necessary.
//
// Example use case, the service detects that it is not configured and wishes to stop running, but does not want
// an error reported, as failing to start may cause commands like `Restart-Service -Force datadogagent` to fail if run
// after modifying the configuration to disable the service.
//
// If your service detects this state in Service.Run() instead then you do not need to do anything, it is handled automatically.
//
// We may be able to remove this and runTimeExitGate if we re-work our current model of change config -> `Restart-Service -Force datadogagent`,
// which we currently expect to trigger all services to restart. Perhaps we can use an agent command instead of PowerShell
// and it can check for a special exit code from each of the services. However we wouldn't be able to use configuration management
// tools' built-in Windows Service commands.
var ErrCleanStopAfterInit = errors.New("the service did not start but requested a clean exit")

// implements golang svc.Handler
type controlHandler struct {
	service Service
}

// This function is a clone of "golang.org/x/sys/windows/svc:IsWindowsService", but with a fix
// for Windows containers. Go cloned the .NET implementation of this function, which has since
// been patched to support Windows containers, which don't use Session ID 0 for services.
// https://github.com/dotnet/runtime/pull/74188
// This function can be replaced with go's once go brings in the fix.
func patchedIsWindowsService() (bool, error) {
	var currentProcess windows.PROCESS_BASIC_INFORMATION
	infoSize := uint32(unsafe.Sizeof(currentProcess))
	err := windows.NtQueryInformationProcess(windows.CurrentProcess(), windows.ProcessBasicInformation, unsafe.Pointer(&currentProcess), infoSize, &infoSize)
	if err != nil {
		return false, err
	}
	var parentProcess *windows.SYSTEM_PROCESS_INFORMATION
	for infoSize = uint32((unsafe.Sizeof(*parentProcess) + unsafe.Sizeof(uintptr(0))) * 1024); ; {
		parentProcess = (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&make([]byte, infoSize)[0]))
		err = windows.NtQuerySystemInformation(windows.SystemProcessInformation, unsafe.Pointer(parentProcess), infoSize, &infoSize)
		if err == nil {
			break
		} else if err != windows.STATUS_INFO_LENGTH_MISMATCH {
			return false, err
		}
	}
	for ; ; parentProcess = (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(uintptr(unsafe.Pointer(parentProcess)) + uintptr(parentProcess.NextEntryOffset))) {
		if parentProcess.UniqueProcessID == currentProcess.InheritedFromUniqueProcessId {
			return strings.EqualFold("services.exe", parentProcess.ImageName.String()), nil
		}
		if parentProcess.NextEntryOffset == 0 {
			break
		}
	}
	return false, nil
}

// RunningAsWindowsService returns true if the current process is running as a Windows Service
// and the application should call Run().
func RunningAsWindowsService() bool {
	isWindowsService, err := patchedIsWindowsService()
	if err != nil {
		fmt.Printf("failed to determine if we are running in an interactive session: %v\n", err)
		return false
	}
	return isWindowsService
}

// Run fullfills the contract required by programs running as Windows Services.
// https://learn.microsoft.com/en-us/windows/win32/services/service-programs
//
// Run should be called as early as possible in the process initialization.
// If called too late you may encounter service start timeout errors from SCM.
// If the process exits without calling this function then SCM will erroneously
// report a timeout error, regardless of how fast the process exits.
//
// SCM only gives services 30 seconds (by default) to respond after the process is created.
// Specifically, this timeout refers to calling StartServiceCtrlDispatcher, which is called by
// golang's svc.Run. This timeout is adjustable at the host level with the ServicesPipeTimeout registry value.
// https://learn.microsoft.com/en-us/troubleshoot/windows-server/system-management-components/service-not-start-events-7000-7011-time-out-error
//
// Golang initializes all packages before giving control to main(). This means that if the package
// initialization takes longer than the SCM timeout then SCM will kill our process before main()
// is even called. One observed source of extended process initialization times is dependency packages that call
// golang's user.Current() function to get the current user. If this becomes a recurring issue we may
// want to consider calling StartServiceCtrlDispatcher before the go runtime is initialized, for example
// via a C constructor.
func Run(service Service) {
	var s controlHandler
	s.service = service

	s.eventlog(messagestrings.MSG_SERVICE_STARTING, s.service.Name())

	// golang svc.Run calls StartServiceCtrlDispatcher, which does not return until the service
	// enters the SERVICE_STOPPED state.
	// golang implements its own ServiceMain function which calls RegisterServiceCtrlHandlerEx, it
	// is up to our Execute() function to handle ChangeRequest's and update SCM with the service status.
	// https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-startservicectrldispatcherw
	// https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-registerservicectrlhandlerexw
	// https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nc-winsvc-lpservice_main_functiona
	// https://learn.microsoft.com/en-us/windows/win32/services/writing-a-service-program-s-main-function
	// https://learn.microsoft.com/en-us/windows/win32/services/writing-a-servicemain-function
	err := svc.Run(s.service.Name(), &s)
	if err != nil {
		s.eventlog(messagestrings.MSG_SERVICE_FAILED, err.Error())
		return
	}

	// svc.Run() can return before Execute() ends if there is an error, but since we trigger
	// process exit when svc.Run() returns its okay if we leak the goroutine.
	s.eventlog(messagestrings.MSG_SERVICE_STOPPED, s.service.Name())
}

// runTimeExitGate is used to ensure the service exits without error on short-lived successful stops by keeping
// the service in the `SERVICE_RUNNING` state long enough for a service manager to consider the start successful.
//
// It should be called when the service enters the `SERVICE_RUNNING` state and should be used to delay
// the service exit until the timer expires.
//
// On Windows, the Service Control Manager (SCM) requires that dependent services stop. This means that when running
// `Restart-Service datadogagent`, Windows will try to stop the Process Agent, and then to be helpful it will immediately start it again.
// However, if Process Agent is not configured to be running it will exit immediately, which `Restart-Service` will report as an error.
// To avoid the error on a successful exit we must ensure that we are in the RUNNING state long enough for `Restart-Service` or other
// tools to consider the restart successful.
//
// See also ErrCleanStopAfterInit
func runTimeExitGate() <-chan time.Time {
	return time.After(5 * time.Second)
}

func (s *controlHandler) eventlog(msgnum uint32, arg string) {
	winutil.LogEventViewer(s.service.Name(), msgnum, arg)
}

// Execute is called by golang svc.Run and is responsible for handling the control requests and state transitions for the service
// golang.org/x/sys/windows/svc contains the actual control handler callback and status handle, and communicates with
// our Execute() function via the provided channels.
// https://learn.microsoft.com/en-us/windows/win32/services/service-status-transitions
func (s *controlHandler) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {

	// first thing we must do is inform SCM that we are SERVICE_START_PENDING.
	// We keep the commands accepted list empty so SCM knows to wait until we start or stop and
	// won't send any signals. This way we don't have to handle stop controls in the middle of starting.
	// https://learn.microsoft.com/en-us/windows/win32/services/service-servicemain-function
	changes <- svc.Status{State: svc.StartPending}

	executeRun := true

	err := s.service.Init()
	if err != nil {
		s.eventlog(messagestrings.MSG_AGENT_START_FAILURE, err.Error())
		if errors.Is(err, ErrCleanStopAfterInit) {
			// Service requested to exit successfully. We must enter SERVICE_RUNNING state and stay there
			// for a period of time to ensure the service manager treats the start as successful.
			// See ErrCleanStopAfterInit and runTimeExitGate for more information.
			// We must still process control requests, in case we receive a STOP signal. If we don't
			// respond to a STOP signal within a few seconds it will fail. So continue and enter
			// RUNNING state and start the control handler, but don't execute Service.Run().
			executeRun = false
		} else {
			return
		}
	}

	// Now tell SCM that we are SERVICE_RUNNING
	// per MSDN: For best system performance, your application should enter the running state within 25-100 milliseconds.
	const runningCmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPreShutdown
	changes <- svc.Status{State: svc.Running, Accepts: runningCmdsAccepted}
	s.eventlog(messagestrings.MSG_SERVICE_STARTED, s.service.Name())

	// make sure that we set state to SERVICE_STOP_PENDING when we return so SCM knows
	// not to send anymore control requests.
	defer func() {
		changes <- svc.Status{State: svc.StopPending}
	}()

	ctx, cancelfunc := context.WithCancel(context.Background())
	defer cancelfunc()

	// goroutine to handle service control requests
	// https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nc-winsvc-lphandler_function
	go s.controlHandlerLoop(cancelfunc, r, changes)

	// Now that we are in SERVICE_RUNNING state, start the exit gate timer.
	exitGate := runTimeExitGate()

	if executeRun {
		// Run the actual agent/service
		err = s.service.Run(ctx)
		if err != nil {
			s.eventlog(messagestrings.MSG_SERVICE_FAILED, err.Error())
			return
		}
	}

	// Run was skipped or returned success, block to ensure the service is alive long enough to be considered successful.
	<-exitGate

	// golang sets the status to SERVICE_STOPPED with ssec,errno before returning from svc.Run()
	// so we don't need to do so here.
	return
}

func (s *controlHandler) controlHandlerLoop(cancelFunc context.CancelFunc, r <-chan svc.ChangeRequest, changes chan<- svc.Status) {
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			// current status query
			changes <- c.CurrentStatus
		case svc.Stop, svc.PreShutdown, svc.Shutdown:
			// Must report SERVICE_STOP_PENDING within a few seconds of receiving the control request
			// or else the service manager may consider the stop a failure.
			changes <- svc.Status{State: svc.StopPending}
			// stop
			s.eventlog(messagestrings.MSG_RECEIVED_STOP_SVC_COMMAND, s.service.Name())
			cancelFunc()
			// We set SERVICE_STOP_PENDING, so SCM won't send anymore control requests
			return
		default:
			// unexpected control
			s.eventlog(messagestrings.MSG_UNEXPECTED_CONTROL_REQUEST, fmt.Sprintf("%d", c.Cmd))
		}
	}
}
