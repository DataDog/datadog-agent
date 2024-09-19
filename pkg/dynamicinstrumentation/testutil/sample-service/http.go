package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
)

func startHTTPServer() error {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Datadog-Response", "Success")
		fmt.Fprint(w, "OK")
	})

	return http.ListenAndServe(":9999", nil)
}

var client = &http.Client{}

func sendHTTPRequest() {
	req, err := http.NewRequest("GET", "http://localhost:9999", nil)
	if err != nil {
		log.Println("Error creating request", err)
		return
	}

	req.Header.Set("X-Datadog-Request", "")
	executeRequest(req)
}

func executeRequest(req *http.Request) {
	resp, err := client.Do(req)
	if err != nil {
		log.Println("HTTP request error", err)
		return
	}
	defer resp.Body.Close()
	processResponse(resp)
}

func processResponse(resp *http.Response) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Error reading response body", err)
	}

	bodyStr := string(body)
	if bodyStr != "OK" {
		log.Println("Unexpected response", bodyStr)
	}
}
