all:
	go build -o build/verity bin/verity.go

test:
	go test
	cd hostname && go test
	cd cpu      && go test
	cd memory   && go test
