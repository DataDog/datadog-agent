package main

import (
	"log"
	"net"
	"net/http"
	"net/rpc"
)

type RuntimeSettingRPCService struct{}

func (svc *RuntimeSettingRPCService) GetLogLevel() (logLevel string, err error) {
	log.Println("recieved logLevel command: get logLevel")
	return
}

func (svc *RuntimeSettingRPCService) SetLogLevel(logLevel string) (err error) {
	log.Println("recieved logLevel command: set logLevel ", logLevel)
	return
}

// StartRuntimeSettingRPCService Starts a runtime server and returns a reference to it
func StartRuntimeSettingRPCService() {
	svc := new(RuntimeSettingRPCService)
	e := rpc.Register(svc)
	if e != nil {
		log.Fatal(e)
	}
	rpc.HandleHTTP()
	l, e := net.Listen("tcp", ":1234")
	if e != nil {
		log.Fatal("listen error:", e)
	}
	e = http.Serve(l, nil)
	if e != nil {
		log.Fatal(e)
	}
}
