package testdns

import (
	"fmt"
	"github.com/miekg/dns"
	"net"
	"os/exec"
	"sync"
	"testing"
)

var globalServer *Server
var globalServerError error
var serverOnce sync.Once

func GetServerIP(t *testing.T) net.IP {
	serverOnce.Do(func() {
		server := NewServer()
		server.Start()
		globalServer = server
	})
	return net.ParseIP("192.1.1.1")
}

type Server struct{}

func NewServer() *Server {
	// TODO: error check?
	c := exec.Command("ip", "link", "add", "dnstestdummy", "type", "dummy")
	c.Run()

	exec.Command("ip", "addr", "add", "dnstestdummy", "dev", "192.1.1.1").Run()
	return nil
}

func (s *Server) Start() {
	started := make(chan struct{}, 1)
	srv := dns.Server{
		Addr: "192.1.1.1:53",
		Net:  "udp",
		Handler: dns.HandlerFunc(func(writer dns.ResponseWriter, msg *dns.Msg) {
			fmt.Println("in handler", msg.Question[0].Name)
			switch msg.Question[0].Name {
			case "good.com.":
				resp := &dns.Msg{}
				resp.SetReply(msg)

				rr, err := dns.NewRR("good.com. 60   IN A 10.0.0.1")
				if err != nil {
					panic(err)
				}
				resp.Answer = []dns.RR{rr}
				writer.WriteMsg(resp)
			default:
				resp := &dns.Msg{}
				resp.SetReply(msg)
				resp.Rcode = dns.RcodeServerFailure
				writer.WriteMsg(resp)

			}
		}),
		NotifyStartedFunc: func() {
			started <- struct{}{}
		},
	}
	go srv.ListenAndServe()
	<-started
}
