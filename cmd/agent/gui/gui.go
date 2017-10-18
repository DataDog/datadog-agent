package gui

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
	"github.com/gorilla/mux"
	"github.com/urfave/negroni"
)

var (
	apiKey   string
	listener net.Listener
)

// Payload struct is for the JSON messages received from a client POST request
type Payload struct {
	Config string `json:"config"`
	Email  string `json:"email"`
	CaseID string `json:"caseID"`
}

// StopGUIServer closes the connection to the HTTP server
func StopGUIServer() {
	if listener != nil {
		listener.Close()
	}
}

// StartGUIServer creates the router and starts the HTTP server
func StartGUIServer(port string) error {
	apiKey = config.Datadog.GetString("api_key")

	// Instantiate the gorilla/mux router
	router := mux.NewRouter()

	if apiKey == "" {
		// If the user doesn't have an API key, they don't serve the GUI
		router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(common.GetViewsPath(), "public/invalidAPI.html"))
		})

		// Serve the only other file needed - the background image
		router.HandleFunc("/view/images/dd_bkgrnd.png", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(common.GetViewsPath(), "public/images/dd_bkgrnd.png"))
		})
	} else {
		// Serve the (secured) index page on the default endpoint
		router.Handle("/", accessAuth(http.FileServer(http.Dir(filepath.Join(common.GetViewsPath(), "private")))))

		// Mount our public filesystem at the view/{path} route
		router.PathPrefix("/view/").Handler(http.StripPrefix("/view/", http.FileServer(http.Dir(filepath.Join(common.GetViewsPath(), "public")))))

		// Mount our secured filesystem at the private/{path} route
		router.PathPrefix("/private/").Handler(http.StripPrefix("/private/", accessAuth(http.FileServer(http.Dir(filepath.Join(common.GetViewsPath(), "private"))))))

		// Set up handlers for the API
		agentRouter := mux.NewRouter().PathPrefix("/agent").Subrouter().StrictSlash(true)
		agentHandler(agentRouter)
		checkRouter := mux.NewRouter().PathPrefix("/checks").Subrouter().StrictSlash(true)
		checkHandler(checkRouter)

		// Add authorization middleware to all the API endpoints
		router.PathPrefix("/agent").Handler(negroni.New(negroni.HandlerFunc(authorize), negroni.Wrap(agentRouter)))
		router.PathPrefix("/checks").Handler(negroni.New(negroni.HandlerFunc(authorize), negroni.Wrap(checkRouter)))
	}

	listener, e := net.Listen("tcp", "127.0.0.1:"+port)
	if e != nil {
		log.Errorf("Error: " + e.Error())
		return e
	}

	go http.Serve(listener, router)
	log.Infof("GUI Server started at %s", listener.Addr())

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
			http.ServeFile(w, r, filepath.Join(common.GetViewsPath(), "public/auth.html"))
		} else {
			h.ServeHTTP(w, r)
		}
	})
}

// Middleware which prevents POST requests from unauthorized clients
func authorize(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
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

	next(w, r)
}

// Helper function which unmarshals a POST requests data into a Payload object
func parseBody(r *http.Request) (Payload, error) {
	var p Payload
	body, e := ioutil.ReadAll(r.Body)
	if e != nil {
		return p, e
	}

	e = json.Unmarshal(body, &p)
	if e != nil {
		return p, e
	}

	return p, nil
}
