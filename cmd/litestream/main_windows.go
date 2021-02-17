// +build windows

package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
)

// serviceName is the Windows Service name.
const serviceName = "Litestream"

// isWindowsService returns true if currently executing within a Windows service.
func isWindowsService() (bool, error) {
	return svc.IsWindowsService()
}

func runWindowsService(ctx context.Context) error {
	elog, err := eventlog.Open(serviceName)
	if err != nil {
		return err
	}
	defer elog.Close()

	// Set eventlog as log writer while running.
	log.SetOutput((*eventlogWriter)(elog))
	defer log.SetOutput(os.Stderr)

	if err := eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil {
		return fmt.Errorf("cannot install event log: %s", err)
	}

	elog.Info(1, "Litestream service starting")

	if err := svc.Run(serviceName, &windowsService{ctx: ctx, elog: elog}); err != nil {
		elog.Error(1, fmt.Sprintf("Litestream service failed: %s", err))
		return errStop
	}
	elog.Info(1, "Litestream service exited")
	return nil
}

// windowsService is an interface adapter for svc.Handler.
type windowsService struct {
	ctx  context.Context
	elog *eventlog.Log
}

func (s *windowsService) Execute(args []string, changeReqCh <-chan svc.ChangeRequest, statusCh chan<- svc.Status) (ssec bool, errno uint32) {
	const accepts = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue

	statusCh <- svc.Status{State: svc.StartPending}

	c := NewReplicateCommand()
	c.Run(s.ctx)

	statusCh <- svc.Status{State: svc.Running, Accepts: accepts}

	for {
		select {
		case changeReq := <-changeReqCh:
			switch changeReq.Cmd {
			case svc.Interrogate:
				s.elog.Info(1, "Litestream service interrograted")
				statusCh <- changeReq.CurrentStatus
			case svc.Stop:
				s.elog.Info(1, "Litestream service stopped")
				c.Close()
				statusCh <- svc.Status{State: svc.StopPending}
			case svc.Shutdown:
				s.elog.Info(1, "Litestream service shutting down")
				c.Close()
				statusCh <- svc.Status{State: svc.StopPending}
			case svc.Pause:
				s.elog.Info(1, "Litestream service paused")
				c.Close()
				statusCh <- svc.Status{State: svc.Paused, Accepts: accepts}
			case svc.Continue:
				s.elog.Info(1, "Litestream service continuing")
				c.Close()
				c = NewReplicateCommand()
				c.Run(s.ctx)
				statusCh <- svc.Status{State: svc.Running, Accepts: accepts}
			default:
				s.elog.Error(1, fmt.Sprintf("unexpected control request #%d", changeReq))
			}
		}
	}
}

// Ensure implementation implements io.Writer interface.
var _ io.Writer = (*eventlogWriter)(nil)

// eventlogWriter is an adapter for using eventlog.Log as an io.Writer.
type eventlogWriter eventlog.Log

func (w *eventlogWriter) Write(p []byte) (n int, err error) {
	return 0, (*eventlog.Log)(w).Info(1, string(p))
}
