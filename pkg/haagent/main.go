package haagent

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"net/http"
	"os"
	"os/signal"

	httpd "github.com/otoolep/hraftd/http"
	"github.com/otoolep/hraftd/store"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Command line defaults
const (
	DefaultHTTPAddr = "localhost:11000"
	DefaultRaftAddr = "localhost:12000"
)

// Command line parameters
var inmem bool
var httpAddr string
var raftAddr string
var joinAddr string
var nodeID string

func initBak() {
	flag.BoolVar(&inmem, "inmem", false, "Use in-memory storage for Raft")
	flag.StringVar(&httpAddr, "haddr", DefaultHTTPAddr, "Set the HTTP bind address")
	flag.StringVar(&raftAddr, "raddr", DefaultRaftAddr, "Set Raft bind address")
	flag.StringVar(&joinAddr, "join", "", "Set join address, if any")
	flag.StringVar(&nodeID, "id", "", "Node ID. If not set, same as Raft bind address")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <raft-data-path> \n", os.Args[0])
		flag.PrintDefaults()
	}
}

func mainBak() {
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "No Raft storage directory specified\n")
		os.Exit(1)
	}

	if nodeID == "" {
		nodeID = raftAddr
	}

	// Ensure Raft storage exists.
	raftDir := flag.Arg(0)
	if raftDir == "" {
		log.Errorf("No Raft storage directory specified")
		return
	}
	if err := os.MkdirAll(raftDir, 0700); err != nil {
		log.Errorf("failed to create path for Raft storage: %s", err.Error())
		return
	}

	s := store.New(inmem)
	s.RaftDir = raftDir
	s.RaftBind = raftAddr
	if err := s.Open(joinAddr == "", nodeID); err != nil {
		log.Errorf("failed to open store: %s", err.Error())
		return
	}

	h := httpd.New(httpAddr, s)
	if err := h.Start(); err != nil {
		log.Errorf("failed to start HTTP service: %s", err.Error())
		return
	}

	// If join was specified, make the join request.
	if joinAddr != "" {
		if err := join(joinAddr, raftAddr, nodeID); err != nil {
			log.Errorf("failed to join node at %s: %s", joinAddr, err.Error())
			return
		}
	}

	// We're up and running!
	log.Infof("hraftd started successfully, listening on http://%s", httpAddr)

	terminate := make(chan os.Signal, 1)
	signal.Notify(terminate, os.Interrupt)
	<-terminate
	log.Infof("hraftd exiting")
}

func join(joinAddr, raftAddr, nodeID string) error {
	b, err := json.Marshal(map[string]string{"addr": raftAddr, "id": nodeID})
	if err != nil {
		return err
	}
	resp, err := http.Post(fmt.Sprintf("http://%s/join", joinAddr), "application-type/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

type HaAgentConfig struct {
	NodeName string `mapstructure:"node_name"` // TODO: use agent hostname instead?
	HttpAddr string `mapstructure:"http_addr"`
	RaftAddr string `mapstructure:"raft_addr"`
	JoinAddr string `mapstructure:"join_addr"`
}

func StartRaft() {
	log.Infof("[HA Agent] Start Raft")
	var haAgentConfig HaAgentConfig
	if err := coreconfig.Datadog.UnmarshalKey("ha_agent", &haAgentConfig); err != nil {
		log.Errorf("unmarshall config: %s", err)
		return
	}
	log.Infof("[HA Agent] Config: %+v", haAgentConfig)

	agentHostname, err := hostname.Get(context.TODO())
	if err != nil {
		log.Errorf("Error getting the hostname: %v", err)
		return
	}
	log.Infof("[HA Agent] hostname: %+v", agentHostname)

	// Ensure Raft storage exists.
	//raftDir := flag.Arg(0)
	//if raftDir == "" {
	//	log.Errorf("No Raft storage directory specified")
	//	return
	//}
	//if err := os.MkdirAll(raftDir, 0700); err != nil {
	//	log.Errorf("failed to create path for Raft storage: %s", err.Error())
	//	return
	//}

	//s := store.New(inmem)
	//s.RaftDir = raftDir
	//s.RaftBind = raftAddr
	//if err := s.Open(joinAddr == "", nodeID); err != nil {
	//	log.Errorf("failed to open store: %s", err.Error())
	//	return
	//}
	//
	//h := httpd.New(httpAddr, s)
	//if err := h.Start(); err != nil {
	//	log.Errorf("failed to start HTTP service: %s", err.Error())
	//	return
	//}
	//
	//// If join was specified, make the join request.
	//if joinAddr != "" {
	//	if err := join(joinAddr, raftAddr, nodeID); err != nil {
	//		log.Errorf("failed to join node at %s: %s", joinAddr, err.Error())
	//		return
	//	}
	//}
	//
	//// We're up and running!
	//log.Infof("hraftd started successfully, listening on http://%s", httpAddr)
	//
	//terminate := make(chan os.Signal, 1)
	//signal.Notify(terminate, os.Interrupt)
	//<-terminate
	//log.Infof("hraftd exiting")
}
