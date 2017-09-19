package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"net/http"

	"github.com/gorilla/mux"
)

var apiKey string

type Message struct {
	Req_type string `json:"req_type"`
	Data     string `json:"data"`
}

func main() {
	e := SetUp()
	if e != nil {
		log.Printf("Error: " + e.Error())
		return
	}

	// Instantiate the gorilla/mux router
	router := mux.NewRouter()

	// Serve the index page on the default endpoint
	router.Handle("/", http.FileServer(http.Dir("./view/")))

	// Mount our filesystem at the view/{path} route
	router.PathPrefix("/view/").Handler(http.StripPrefix("/view/", http.FileServer(http.Dir("./view/"))))

	// Handle requests from clients
	router.Handle("/req", authenticate(http.HandlerFunc(handler)))

	// TODO: in SetUp, read datadog.yaml to see if user specified a different port to use

	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Printf("Error: " + err.Error())
		return
	}
	http.Serve(listener, router)
}

// authenticate is middleware which prevents requests unauthorized client requests from getting through
func authenticate(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if r.Method != "POST" {
			w.Write([]byte("Error: only POST requests are accepted."))
			return
		}

		clientKey := r.Header["Authorization"]
		if len(clientKey) == 0 || clientKey[0] == "" {
			w.Write([]byte("Error: no API key present."))
			return
		} else if clientKey[0] != "Bearer "+apiKey {
			w.Write([]byte("Error: invalid API key."))
			return
		}

		h.ServeHTTP(w, r)
	})
}

// handler receives POST requests from the front end
func handler(w http.ResponseWriter, r *http.Request) {
	// Decode the data from the request
	body, e := ioutil.ReadAll(r.Body)
	if e != nil {
		log.Println("Error: " + e.Error())
		w.Write([]byte("Error: " + e.Error()))
		return
	}
	var m Message
	e = json.Unmarshal(body, &m)
	if e != nil {
		log.Println("Error: " + e.Error())
		w.Write([]byte("Error: " + e.Error()))
		return
	}

	// Make sure message received was the correct format
	if m.Req_type == "" || m.Data == "" {
		w.Write([]byte("Invalid message received: incorrect format."))
		log.Printf("Invalid message received: incorrect format.")
		return
	}

	switch m.Req_type {
	case "command":
		ProcessCommand(w, m.Data)
	default:
		// other types of messages will go here
	}
}
