package main

import (
	"fmt"
	"log"
	"net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Fake Redis")
}

func main() {
	http.HandleFunc("/", handler)
	log.Fatal(http.ListenAndServe(":56379", nil))
}
