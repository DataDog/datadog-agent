all:
	go build -o build/verity bin/verity.go

test:
	go test
	cd cpu         && go test
	cd env         && go test
	cd hostname    && go test
	cd ipaddress   && go test
	cd ipv6address && go test
	cd macaddress  && go test
	cd memory      && go test
	cd network     && go test
