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

// Message struct is for the JSON messages received from a client POST request
type Message struct {
	ReqType string `json:"req_type"`
	Data    string `json:"data"`
	Payload string `json:"payload"`
}

// StopGUIServer closes the connection to the HTTP server
func StopGUIServer() {
	if listener != nil {
		listener.Close()
	}
}

// StartGUIServer creates the router and starts the HTTP server
func StartGUIServer() error {
	apiKey = config.Datadog.GetString("api_key")
	port := ":" + config.Datadog.GetString("GUI_port")

	if config.Datadog.GetString("GUI_port") == "-1" {
		log.Infof("Port -1 specified: not starting the GUI server.")
		return nil
	}

	// Instantiate the gorilla/mux router
	router := mux.NewRouter()

	// Serve the (secured) index page on the default endpoint
	router.Handle("/", accessAuth(http.FileServer(http.Dir("view/private/"))))

	// Mount our public filesystem at the view/{path} route
	router.PathPrefix("/view/").Handler(http.StripPrefix("/view/", http.FileServer(http.Dir("view/public/"))))

	// Mount our secured filesystem at the private/{path} route
	router.PathPrefix("/private/").Handler(http.StripPrefix("/private/", accessAuth(http.FileServer(http.Dir("view/private/")))))

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

// Middleware which blocks access to secured files by serving the auth page if the client is not authenticated
func accessAuth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, _ := r.Cookie("token")

		// Disable caching to ensure client loads the correct file
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

		if cookie == nil || cookie.Value != apiKey {
			// Serve the authentication page
			http.ServeFile(w, r, "view/public/auth.html")
		} else {
			h.ServeHTTP(w, r)
		}
	})
}

// Middleware which prevents POST requests from unauthorized clients
func authenticate(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientToken := r.Header["Authorization"]

		// If the client has an incorrect authorization scheme, reply with a 401 (Unauthorized) response
		if len(clientToken) == 0 || clientToken[0] == "" || strings.Split(clientToken[0], " ")[0] != "Bearer" {
			w.Header().Set("WWW-Authenticate", "Bearer realm=\"Access to Datadog Agent Manager\"")
			e := fmt.Errorf("invalid authorization scheme")
			http.Error(w, e.Error(), 401)
			log.Infof("GUI - Received unauthorized request (invalid scheme).")
			return
		}

		// If they don't have the correct apiKey, send a 403 (Forbidden) response
		if clientToken = strings.Split(clientToken[0], " "); apiKey != "" && clientToken[1] != apiKey {
			e := fmt.Errorf("invalid authorization token")
			http.Error(w, e.Error(), 403)
			log.Infof("GUI - Received unauthorized request (bad token).")
			return
		}

		h.ServeHTTP(w, r)
	})
}

// Handler for all authorized POST requests
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
	if m.ReqType == "" || m.Data == "" {
		w.Write([]byte("Invalid message received: incorrect format."))
		log.Infof("GUI - Invalid message received: incorrect format.")
		return
	}

	switch m.ReqType {

	case "fetch":
		fetch(w, m)

	case "set":
		set(w, m)

	case "ping":
		w.Write([]byte("Pong"))

	default:
		w.Write([]byte("Received unknown request type: " + m.ReqType))
		log.Infof("GUI - Received unknown request type: %v ", m.ReqType)
	}
}
