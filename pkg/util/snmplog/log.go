package snmplog

import (
	"fmt"

	ddlog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
)

var (
	logCtx *SNMPLogger
)

type (
	SNMPLogger struct {
		Device string
		IP     string
	}
)

func SetupSNMPLogger(device, ip string) {
	logCtx = &SNMPLogger{
		Device: device,
		IP:     ip,
	}
}

func (s *SNMPLogger) getFormat() string {
	return fmt.Sprintf("Device %s | IP %s | ", s.Device, s.IP)
}

// Debug logs at the debug level
func Debug(v ...interface{}) {
	if logCtx != nil {
		ddlog.Debug(logCtx.getFormat(), v)
	} else {
		ddlog.Debug(v...)
	}
}

// Debugf logs with format at the debug level
func Debugf(format string, params ...interface{}) {
	if logCtx != nil {
		ddlog.Debugf(fmt.Sprintf("%s %s", logCtx.getFormat(), format), params...)
	} else {
		ddlog.Debugf(format, params...)
	}
}

func Info(v ...interface{}) {
	if logCtx != nil {
		ddlog.Info(logCtx.getFormat(), v)
	} else {
		ddlog.Info(v...)
	}
}

func Infof(format string, params ...interface{}) {
	if logCtx != nil {
		ddlog.Infof(fmt.Sprintf("%s %s", logCtx.getFormat(), format), params...)
	} else {
		ddlog.Infof(format, params...)
	}
}

func Warn(v ...interface{}) error {
	var err error
	if logCtx != nil {
		err = ddlog.Warn(logCtx.getFormat(), v)
	} else {
		err = ddlog.Warn(v...)
	}

	return err
}

func Warnf(format string, params ...interface{}) {
	if logCtx != nil {
		ddlog.Warnf(fmt.Sprintf("%s %s", logCtx.getFormat(), format), params...)
	} else {
		ddlog.Warnf(format, params...)
	}
}

func Trace(v ...interface{}) {
	if logCtx != nil {
		ddlog.Trace(logCtx.getFormat(), v)
	} else {
		ddlog.Trace(v...)
	}
}

func Tracef(format string, params ...interface{}) {
	if logCtx != nil {
		ddlog.Tracef(fmt.Sprintf("%s %s", logCtx.getFormat(), format), params...)
	} else {
		ddlog.Tracef(format, params...)
	}
}

func Error(v ...interface{}) {
	if logCtx != nil {
		ddlog.Error(logCtx.getFormat(), v)
	} else {
		ddlog.Error(v...)
	}
}

func Errorf(format string, params ...interface{}) {
	if logCtx != nil {
		ddlog.Errorf(fmt.Sprintf("%s %s", logCtx.getFormat(), format), params...)
	} else {
		ddlog.Errorf(format, params...)
	}
}

func ShouldLog(lvl seelog.LogLevel) bool {
	return ddlog.ShouldLog(lvl)
}
