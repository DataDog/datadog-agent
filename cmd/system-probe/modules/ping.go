package modules

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	pingcheck "github.com/DataDog/datadog-agent/pkg/networkdevice/pinger"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/gorilla/mux"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
)

type pinger struct {
	config *pingcheck.Config
}

// Pinger is a factory for NDMs Ping module
var Pinger = module.Factory{
	Name:             config.PingModule,
	ConfigNamespaces: []string{"ping_config"},
	Fn: func(cfg *config.Config) (module.Module, error) {
		return &pinger{}, nil
	},
}

var _ module.Module = &pinger{}

func (p *pinger) GetStats() map[string]interface{} {
	return nil
}

func (p *pinger) Register(httpMux *module.Router) error {
	var runCounter = atomic.NewUint64(0)

	httpMux.HandleFunc("/ping/{host}", utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests, func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		vars := mux.Vars(req)
		id := getClientID(req)
		host := vars["host"]

		// TODO:(ken) read in config from system probe config and/or regular config
		// read it in once to the pinger struct, pass it down here
		cfg := pingcheck.Config{
			UseRawSocket: true,
		}

		// Run ping using raw socket
		result, err := pingcheck.RunPing(&cfg, host)
		if err != nil {
			log.Errorf("unable to run ping for host %s: %s", host, err)
			w.WriteHeader(500)
			return
		}

		resp, err := json.Marshal(result)
		if err != nil {
			log.Errorf("unable to marshall ping stats: %s", err)
			w.WriteHeader(500)
			return
		}
		_, err = w.Write(resp)
		if err != nil {
			log.Errorf("unable to write ping response: %s", err)
		}

		count := runCounter.Inc()
		logPingRequests(host, id, count, start, result)
	}))

	return nil
}

func (p *pinger) RegisterGRPC(_ grpc.ServiceRegistrar) error {
	return nil
}

func (p *pinger) Close() {}

func logPingRequests(host string, client string, count uint64, start time.Time, result *pingcheck.Result) {
	args := []interface{}{host, client, count, time.Since(start), result}
	msg := "Got request on /ping/%s?client_id=%s (count: %d): retrieved ping in %s: result: %+v"
	switch {
	case count <= 5, count%20 == 0:
		log.Infof(msg, args...)
	default:
		log.Debugf(msg, args...)
	}
}
