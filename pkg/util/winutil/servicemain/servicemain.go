// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package servicemain

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/messagestrings"

	"golang.org/x/sys/windows/svc"
)

// implements golang svc.Handler
type controlHandler struct {
	serviceName string
	service     func(ctx context.Context) error
}

// RunningAsWindowsService returns true if the current process is running as a Windows Service
// and the application should call RunAsWindowsService.
func RunningAsWindowsService() bool {
	isWindowsService, err := svc.IsWindowsService()
	if err != nil {
		fmt.Printf("failed to determine if we are running in an interactive session: %v\n", err)
		return false
	}
	return isWindowsService
}

// RunAsWindowsService fullfills the contract required by programs running as Windows Services.
// https://learn.microsoft.com/en-us/windows/win32/services/service-programs
//
// RunAsWindowsService should be called as early as possible in the process initialization.
// If called too late you may encounter service start timeout errors from SCM.
//
// SCM only gives services 30 seconds (by default) to respond after the process is created.
// Specifically, this timeout refers to calling StartServiceCtrlDispatcher, which is called by
// golang's svc.Run.
// This timeout is adjustible at the host level with the ServicesPipeTimeout registry value.
// https://learn.microsoft.com/en-us/troubleshoot/windows-server/system-management-components/service-not-start-events-7000-7011-time-out-error
//
// Golang initializes all packages before giving control to main(). This means that if the package
// initialization takes longer than the SCM timeout then SCM will kill our process before main()
// is even called.
//
// One observed source of extended process initialization times is dependency packages that call
// golang's user.Current() function to get the current user. If this becomes a recurring issue we may
// want to consider calling StartServiceCtrlDispatcher before the go runtime is initialized, for example
// via a C constructor.
//
// @serviceName
//   - used as the source for event log records.
//
// @service
//   - Function containing all application logic when run as a service.
//   - The context argument to the @service function is cancellable. The @service must monitor
//     ctx.Done() and return as soon as possible when it is set. There is no standard time limit, it
//     is up to the program performing the service stop. The Services.msc snap-in has a 125 second limit,
//     if the operating system is rebooting there is a 20 second limit.
//     https://learn.microsoft.com/en-us/windows/win32/services/service-control-handler-function
//   - The @service function must return. If it does not return then the service status will not
//     be updated to SERVICE_STOPPED. Service Control Manager (SCM) will treat this as an unexpected exit
//     and enter failure/recovery, regardless of the process exit code.
//     https://learn.microsoft.com/en-us/windows/win32/api/winsvc/ns-winsvc-service_failure_actionsa
func RunAsWindowsService(serviceName string, service func(ctx context.Context) error) {
	var s controlHandler
	s.serviceName = serviceName
	s.service = service

	s.eventlog(messagestrings.MSG_SERVICE_STARTING, s.serviceName)

	// golang svc.Run calls StartServiceCtrlDispatcher, which does not return until the service
	// enters the SERVICE_STOPPED state.
	// golang implements its own ServiceMain function which calls RegisterServiceCtrlHandlerEx, it
	// is up to our Execute() function to handle ChangeRequest's and update SCM with the service status.
	// https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-startservicectrldispatcherw
	// https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-registerservicectrlhandlerexw
	// https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nc-winsvc-lpservice_main_functiona
	// https://learn.microsoft.com/en-us/windows/win32/services/writing-a-service-program-s-main-function
	// https://learn.microsoft.com/en-us/windows/win32/services/writing-a-servicemain-function
	err := svc.Run(s.serviceName, &s)
	if err != nil {
		s.eventlog(messagestrings.MSG_SERVICE_FAILED, err.Error())
		return
	}

	// svc.Run() can return before Execute() ends if there is an error, but since we trigger
	// process exit when svc.Run() returns its okay if we leak the goroutine.
	s.eventlog(messagestrings.MSG_SERVICE_STOPPED, s.serviceName)
}

// If the service depends on datadogagent, then defer this function in your RunAsWindowService callback.
//
// On Windows, the Service Control Manager (SCM) requires that dependent services stop. This means that when running
// `Restart-Service datadogagent`, Windows will try to stop the Process Agent, and then to be helpful it will immediately start it again.
// However, if Process Agent is not configured to be running it will exit immediately, which `Restart-Service` will report as an error.
// To avoid the error on a successful exit we must ensure that we are in the RUNNING state long enough for `Restart-Service` or other
// tools to consider the restart successful.
func RunTimeExitGate() {
	exitGate := time.After(5 * time.Second)
	<-exitGate
}

func (s *controlHandler) eventlog(msgnum uint32, arg string) {
	winutil.LogEventViewer(s.serviceName, msgnum, arg)
}

// Execute is called by golang svc.Run and is responisble for handling the control requests and state transitions for the service
// https://learn.microsoft.com/en-us/windows/win32/services/service-status-transitions
func (s *controlHandler) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	// first thing we must do is inform SCM that we are SERVICE_START_PENDING.
	changes <- svc.Status{State: svc.StartPending}

	// Now tell SCM that we are SERVICE_RUNNING
	// per MSDN: For best system performance, your application should enter the running state within 25-100 milliseconds.
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPreShutdown
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	s.eventlog(messagestrings.MSG_SERVICE_STARTED, s.serviceName)

	// make sure that we set state to SERVICE_STOP_PENDING when we return so SCM knows
	// not to send anymore control requests.
	defer func() {
		changes <- svc.Status{State: svc.StopPending}
	}()

	ctx, cancelfunc := context.WithCancel(context.Background())

	// goroutine to handle service control requests
	// https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nc-winsvc-lphandler_function
	go func() {
		for c := range r {
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.PreShutdown, svc.Shutdown:
				s.eventlog(messagestrings.MSG_RECEIVED_STOP_SVC_COMMAND, s.serviceName)
				cancelfunc()
				return
			default:
				// unexpected control
				s.eventlog(messagestrings.MSG_UNEXPECTED_CONTROL_REQUEST, fmt.Sprintf("%d", c.Cmd))
			}
		}
	}()

	// Run the actual agent/service
	err := s.service(ctx)
	if err != nil {
		s.eventlog(messagestrings.MSG_SERVICE_FAILED, err.Error())
	}

	// golang sets the status to SERVICE_STOPPED before returning from svc.Run() so we don't
	// need to do so here.
	return
}
