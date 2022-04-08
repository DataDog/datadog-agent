package api

// UpdateLogLevel updates HTTPReceiver unexported fields according to the
// updated log level of trace agent
func (r *HTTPReceiver) UpdateLogLevel(logLvl string) {
	r.conf.LogLevel = logLvl
	r.debug = logLvl == "debug"
}
