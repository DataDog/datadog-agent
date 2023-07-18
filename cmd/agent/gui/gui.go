// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package gui

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/urfave/negroni"

	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	listener  net.Listener
	authToken string

	// CsrfToken is a session-specific token passed to the GUI's authentication endpoint by app.launchGui
	CsrfToken string

	// To compute uptime
	startTimestamp int64
)

//go:embed views
var viewsFS embed.FS

// Payload struct is for the JSON messages received from a client POST request
type Payload struct {
	Config string `json:"config"`
	Email  string `json:"email"`
	CaseID string `json:"caseID"`
}

// StopGUIServer closes the connection to the HTTP server & removes the authentication token file we created
func StopGUIServer() {
	if listener != nil {
		listener.Close()
	}
}

// StartGUIServer creates the router, starts the HTTP server & generates the authentication token for access
func StartGUIServer(port string, flare flare.Component) error {
	// Set start time...
	startTimestamp = time.Now().Unix()

	// Instantiate the gorilla/mux router
	router := mux.NewRouter()

	// Serve the only public file at the authentication endpoint
	router.HandleFunc("/authenticate", generateAuthEndpoint)

	// Serve the (secured) index page on the default endpoint
	router.Handle("/", authorizeAccess(http.HandlerFunc(generateIndex)))

	// Mount our (secured) filesystem at the view/{path} route
	router.PathPrefix("/view/").Handler(http.StripPrefix("/view/", authorizeAccess(http.HandlerFunc(serveAssets))))

	// Set up handlers for the API
	agentRouter := mux.NewRouter().PathPrefix("/agent").Subrouter().StrictSlash(true)
	agentHandler(agentRouter, flare)
	checkRouter := mux.NewRouter().PathPrefix("/checks").Subrouter().StrictSlash(true)
	checkHandler(checkRouter)

	// Add authorization middleware to all the API endpoints
	router.PathPrefix("/agent").Handler(negroni.New(negroni.HandlerFunc(authorizePOST), negroni.Wrap(agentRouter)))
	router.PathPrefix("/checks").Handler(negroni.New(negroni.HandlerFunc(authorizePOST), negroni.Wrap(checkRouter)))

	// Listen & serve
	listener, e := net.Listen("tcp", "127.0.0.1:"+port)
	if e != nil {
		return e
	}
	go http.Serve(listener, router) //nolint:errcheck
	log.Infof("GUI server is listening at 127.0.0.1:" + port)

	// Create a CSRF token (unique to each session)
	e = createCSRFToken()
	if e != nil {
		return e
	}

	// Fetch the authentication token (persists across sessions)
	authToken, e = security.FetchAuthToken()
	if e != nil {
		listener.Close()
		listener = nil
	}
	return e
}

func createCSRFToken() error {
	key := make([]byte, 32)
	_, e := rand.Read(key)
	if e != nil {
		return fmt.Errorf("error creating CSRF token: " + e.Error())
	}
	CsrfToken = hex.EncodeToString(key)
	return nil
}

func generateIndex(w http.ResponseWriter, r *http.Request) {
	data, err := viewsFS.ReadFile("views/templates/index.tmpl")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	t, e := template.New("index.tmpl").Parse(string(data))
	if e != nil {
		http.Error(w, e.Error(), http.StatusInternalServerError)
		return
	}

	e = t.Execute(w, map[string]bool{"restartEnabled": restartEnabled()})
	if e != nil {
		http.Error(w, e.Error(), http.StatusInternalServerError)
		return
	}
}

func generateAuthEndpoint(w http.ResponseWriter, r *http.Request) {
	data, err := viewsFS.ReadFile("views/templates/auth.tmpl")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	t, e := template.New("auth.tmpl").Parse(string(data))
	if e != nil {
		http.Error(w, e.Error(), http.StatusInternalServerError)
		return
	}

	e = t.Execute(w, map[string]interface{}{"csrf": CsrfToken})
	if e != nil {
		http.Error(w, e.Error(), http.StatusInternalServerError)
		return
	}
}

func serveAssets(w http.ResponseWriter, req *http.Request) {
	path := path.Join("views", "private", req.URL.Path)
	data, err := viewsFS.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	ctype := mime.TypeByExtension(filepath.Ext(path))
	if ctype == "" {
		ctype = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

// Middleware which blocks access to secured files from unauthorized clients
func authorizeAccess(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Disable caching
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

		cookie, _ := r.Cookie("authToken")
		if cookie == nil {
			w.WriteHeader(http.StatusUnauthorized)
			http.Error(w, "no authorization token", 401)
			return
		}

		if cookie.Value != authToken {
			w.WriteHeader(http.StatusUnauthorized)
			http.Error(w, "invalid authorization token", 401)
			return
		}

		// Token was valid: serve the requested resource
		h.ServeHTTP(w, r)
	})
}

// Middleware which blocks POST requests from unauthorized clients
func authorizePOST(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	authHeader := r.Header["Authorization"]
	if len(authHeader) == 0 || authHeader[0] == "" || strings.Split(authHeader[0], " ")[0] != "Bearer" {
		w.WriteHeader(http.StatusUnauthorized)
		http.Error(w, "invalid authorization scheme", 401)
		return
	}

	token := strings.Split(authHeader[0], " ")[1]
	if token != authToken {
		w.WriteHeader(http.StatusUnauthorized)
		http.Error(w, "invalid authorization token", 401)
		return
	}

	next(w, r)
}

// Helper function which unmarshals a POST requests data into a Payload object
func parseBody(r *http.Request) (Payload, error) {
	var p Payload
	body, e := io.ReadAll(r.Body)
	if e != nil {
		return p, e
	}

	e = json.Unmarshal(body, &p)
	if e != nil {
		return p, e
	}

	return p, nil
}
