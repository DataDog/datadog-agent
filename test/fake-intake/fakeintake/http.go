package fakeintake

import "net/http"

type httpResponse struct {
	contentType string
	statusCode  int
	body        []byte
}

func writeHttpResponse(w http.ResponseWriter, response httpResponse) {
	if response.contentType != "" {
		w.Header().Set("Content-Type", response.contentType)
	}
	w.WriteHeader(response.statusCode)
	if len(response.body) > 0 {
		w.Write(response.body)
	}
}
