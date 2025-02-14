#!/usr/bin/make

main: main.go ../events.go
	go build -tags "linux,linux_bpf,test" -o main main.go

clean:
	rm -f main
