package modules

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/gorilla/mux"
	probing "github.com/prometheus-community/pro-bing"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
)

type pinger struct{}

type jsonStats struct {
	// PacketsRecv is the number of packets received.
	PacketsRecv int `json:"packetsRecv"`

	// PacketsSent is the number of packets sent.
	PacketsSent int `json:"packetsSent"`

	// PacketsRecvDuplicates is the number of duplicate responses there were to a sent packet.
	PacketsRecvDuplicates int `json:"packetsRecvDuplicates"`

	// PacketLoss is the percentage of packets lost.
	PacketLoss float64 `json:"packetLoss"`

	// IPAddr is the address of the host being pinged.
	IPAddr string `json:"ipAddress"`

	// Addr is the string address of the host being pinged.
	Addr string `json:"host"`

	// Rtts is all of the round-trip times sent via this pinger.
	Rtts []time.Duration `json:"rtts"`

	// MinRtt is the minimum round-trip time sent via this pinger.
	MinRtt time.Duration `json:"minRtt"`

	// MaxRtt is the maximum round-trip time sent via this pinger.
	MaxRtt time.Duration `json:"maxRtt"`

	// AvgRtt is the average round-trip time sent via this pinger.
	AvgRtt time.Duration `json:"avgRtt"`

	// StdDevRtt is the standard deviation of the round-trip times sent via
	// this pinger.
	StdDevRtt time.Duration `json:"StdDevRtt"`
}

// Pinger is a factory for NDMs Ping module
var Pinger = module.Factory{
	Name:             config.PingerModule,
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

		stats, err := runPing(host)
		if err != nil {
			log.Errorf("unable to run ping for host %s: %s", host, err)
			w.WriteHeader(500)
			return
		}

		resp, err := json.Marshal(stats)
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
		logPingRequests(host, id, count, start)
	}))

	return nil
}

func (p *pinger) RegisterGRPC(_ grpc.ServiceRegistrar) error {
	return nil
}

func (p *pinger) Close() {}

func runPing(host string) (jsonStats, error) {
	pinger, err := probing.NewPinger(host)
	if err != nil {
		return jsonStats{}, err
	}
	pinger.Count = 3
	err = pinger.Run() // Blocks until finished.
	if err != nil {
		return jsonStats{}, err
	}
	stats := pinger.Statistics() // get send/receive/duplicate/rtt stats

	return jsonStats{
		PacketsRecv:           stats.PacketsRecv,
		PacketsSent:           stats.PacketsSent,
		PacketsRecvDuplicates: stats.PacketsRecvDuplicates,
		PacketLoss:            stats.PacketLoss,
		IPAddr:                stats.IPAddr.String(),
		Addr:                  stats.Addr,
		Rtts:                  stats.Rtts,
		MinRtt:                stats.MinRtt,
		MaxRtt:                stats.MaxRtt,
		AvgRtt:                stats.AvgRtt,
		StdDevRtt:             stats.StdDevRtt,
	}, nil
}

func logPingRequests(host string, client string, count uint64, start time.Time) {
	args := []interface{}{host, client, count, time.Since(start)}
	msg := "Got request on /ping/%s?client_id=%s (count: %d): retrieved ping in %s"
	switch {
	case count <= 5, count%20 == 0:
		log.Infof(msg, args...)
	default:
		log.Debugf(msg, args...)
	}
}
