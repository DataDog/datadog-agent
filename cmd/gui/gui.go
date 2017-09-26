package gui

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
	"github.com/gorilla/mux"
)

var (
	apiKey   string
	listener net.Listener
)

type Message struct {
	Req_type string `json:"req_type"`
	Data     string `json:"data"`
	Payload  string `json:"payload"`
}

func StopGUIServer() {
	if listener != nil {
		listener.Close()
	}
}

func StartGUIServer() error {
	log.Infof("GUI - Setting up server...")

	apiKey = config.Datadog.GetString("api_key")
	port := ":" + config.Datadog.GetString("GUI_port")

	// Instantiate the gorilla/mux router
	router := mux.NewRouter()

	// Serve the index page on the default endpoint
	router.Handle("/", http.FileServer(http.Dir("view/")))

	// Mount our filesystem at the view/{path} route
	router.PathPrefix("/view/").Handler(http.StripPrefix("/view/", http.FileServer(http.Dir("view/"))))

	// Handle requests from clients
	router.Handle("/req", authenticate(http.HandlerFunc(handler))).Methods("POST")

	listener, e := net.Listen("tcp", port)
	if e != nil {
		log.Errorf("Error: " + e.Error())
		return e
	}

	go http.Serve(listener, router)

	log.Infof("GUI - Server started.")
	return nil
}

// Middleware which prevents requests unauthorized client requests from getting through
func authenticate(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientToken := r.Header["Authorization"]

		// If the client has an incorrect authorization scheme, reply with a 401 (Unauthorized) response
		if len(clientToken) == 0 || clientToken[0] == "" || strings.Split(clientToken[0], " ")[0] != "Bearer" {
			w.Header().Set("WWW-Authenticate", "Bearer realm=\"Access to Datadog Agent Manager\"")
			e := fmt.Errorf("invalid authorization scheme.")
			http.Error(w, e.Error(), 401)
			return
		}

		// If they don't have the correct apiKey, send a 403 (Forbidden) response
		if clientToken = strings.Split(clientToken[0], " "); clientToken[1] != apiKey {
			e := fmt.Errorf("invalid authorization token.")
			http.Error(w, e.Error(), 403)
			return
		}

		h.ServeHTTP(w, r)
	})
}

func handler(w http.ResponseWriter, r *http.Request) {
	// Decode the data from the request
	body, e := ioutil.ReadAll(r.Body)
	if e != nil {
		log.Errorf("Error: " + e.Error())
		w.Write([]byte("Error: " + e.Error()))
		return
	}
	var m Message
	e = json.Unmarshal(body, &m)
	if e != nil {
		log.Errorf("Error: " + e.Error())
		w.Write([]byte("Error: " + e.Error()))
		return
	}

	// Make sure message received was the correct format
	if m.Req_type == "" || m.Data == "" {
		w.Write([]byte("Invalid message received: incorrect format."))
		log.Infof("Invalid message received: incorrect format.")
		return
	}

	switch m.Req_type {

	case "fetch":
		fetch(w, m.Data)

	case "set":
		set(w, m.Data, m.Payload)

	case "check":
		check(w, m.Data, m.Payload)

	case "ping":
		w.Write([]byte("Pong"))

	default:
		w.Write([]byte("Received unknown request type: " + m.Req_type))
		log.Infof("GUI - Received unknown request type: %v ", m.Req_type)
	}
}
