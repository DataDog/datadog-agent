package main

import (
	"fmt"
	"log"
	"net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Fake Consul")
}

func main() {
	http.HandleFunc("/", handler)
	log.Fatal(http.ListenAndServe(":58500", nil))
}
