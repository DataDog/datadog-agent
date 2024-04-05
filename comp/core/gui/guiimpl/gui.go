// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package guiimpl

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"go.uber.org/fx"

	"github.com/gorilla/mux"
	"github.com/urfave/negroni"

	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	guicomp "github.com/DataDog/datadog-agent/comp/core/gui"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/api/security"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newGui),
	)
}

type gui struct {
	logger log.Component

	port      string
	listener  net.Listener
	router    *mux.Router
	authToken string

	// CsrfToken is a session-specific token passed to the GUI's authentication endpoint by app.launchGui
	CsrfToken string

	// To compute uptime
	startTimestamp int64
}

//go:embed views
var viewsFS embed.FS

// Payload struct is for the JSON messages received from a client POST request
type Payload struct {
	Config string `json:"config"`
	Email  string `json:"email"`
	CaseID string `json:"caseID"`
}

type dependencies struct {
	fx.In

	Log       log.Component
	Config    config.Component
	Flare     flare.Component
	Status    status.Component
	Collector collector.Component
	Ac        autodiscovery.Component
	Lc        fx.Lifecycle
}

// GUI component implementation constructor
// @param deps dependencies needed to construct the gui, bundled in a struct
// @return an optional, depending of "GUI_port" configuration value
func newGui(deps dependencies) optional.Option[guicomp.Component] {

	guiPort := deps.Config.GetString("GUI_port")

	if guiPort == "-1" {
		deps.Log.Infof("GUI server port -1 specified: not starting the GUI.")
		return optional.NewNoneOption[guicomp.Component]()
	}

	g := gui{
		port:   guiPort,
		logger: deps.Log,
	}

	// Create a CSRF token (unique to each session)
	e := g.createCSRFToken()
	if e != nil {
		g.logger.Errorf("GUI server initialization failed (unable to create CSRF token): ", e)
		return optional.NewNoneOption[guicomp.Component]()
	}

	// Fetch the authentication token (persists across sessions)
	g.authToken, e = security.FetchAuthToken(deps.Config)
	if e != nil {
		g.logger.Errorf("GUI server initialization failed (unable to get the AuthToken): ", e)
		return optional.NewNoneOption[guicomp.Component]()
	}

	// Instantiate the gorilla/mux router
	router := mux.NewRouter()

	// Serve the only public file at the authentication endpoint
	router.HandleFunc("/authenticate", g.generateAuthEndpoint)

	// Serve the (secured) index page on the default endpoint
	router.Handle("/", g.authorizeAccess(http.HandlerFunc(generateIndex)))

	// Mount our (secured) filesystem at the view/{path} route
	router.PathPrefix("/view/").Handler(http.StripPrefix("/view/", g.authorizeAccess(http.HandlerFunc(serveAssets))))

	// Set up handlers for the API
	agentRouter := mux.NewRouter().PathPrefix("/agent").Subrouter().StrictSlash(true)
	agentHandler(agentRouter, deps.Flare, deps.Status, deps.Config, g.startTimestamp)
	checkRouter := mux.NewRouter().PathPrefix("/checks").Subrouter().StrictSlash(true)
	checkHandler(checkRouter, deps.Collector, deps.Ac)

	// Add authorization middleware to all the API endpoints
	router.PathPrefix("/agent").Handler(negroni.New(negroni.HandlerFunc(g.authorizePOST), negroni.Wrap(agentRouter)))
	router.PathPrefix("/checks").Handler(negroni.New(negroni.HandlerFunc(g.authorizePOST), negroni.Wrap(checkRouter)))

	g.router = router

	deps.Lc.Append(fx.Hook{
		OnStart: g.start,
		OnStop:  g.stop})

	return optional.NewOption[guicomp.Component](g)
}

// start function is provided to fx as OnStart lifecycle hook, it run the GUI server
func (g *gui) start(_ context.Context) error {
	var e error

	// Set start time...
	g.startTimestamp = time.Now().Unix()

	g.listener, e = net.Listen("tcp", "127.0.0.1:"+g.port)
	if e != nil {
		g.logger.Errorf("GUI server didn't achieved to start: ", e)
		return nil
	}
	go http.Serve(g.listener, g.router) //nolint:errcheck
	g.logger.Infof("GUI server is listening at 127.0.0.1:" + g.port)
	return nil
}

func (g *gui) stop(_ context.Context) error {
	if g.listener != nil {
		g.listener.Close()
	}
	return nil
}

func (g *gui) createCSRFToken() error {
	key := make([]byte, 32)
	_, e := rand.Read(key)
	if e != nil {
		return fmt.Errorf("error creating CSRF token: " + e.Error())
	}
	g.CsrfToken = hex.EncodeToString(key)
	return nil
}

func generateIndex(w http.ResponseWriter, _ *http.Request) {
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

func (g *gui) generateAuthEndpoint(w http.ResponseWriter, _ *http.Request) {
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

	e = t.Execute(w, map[string]interface{}{"csrf": g.CsrfToken})
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
func (g *gui) authorizeAccess(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Disable caching
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

		cookie, _ := r.Cookie("authToken")
		if cookie == nil {
			w.WriteHeader(http.StatusUnauthorized)
			http.Error(w, "no authorization token", 401)
			return
		}

		if cookie.Value != g.authToken {
			w.WriteHeader(http.StatusUnauthorized)
			http.Error(w, "invalid authorization token", 401)
			return
		}

		// Token was valid: serve the requested resource
		h.ServeHTTP(w, r)
	})
}

// Middleware which blocks POST requests from unauthorized clients
func (g *gui) authorizePOST(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	authHeader := r.Header["Authorization"]
	if len(authHeader) == 0 || authHeader[0] == "" || strings.Split(authHeader[0], " ")[0] != "Bearer" {
		w.WriteHeader(http.StatusUnauthorized)
		http.Error(w, "invalid authorization scheme", 401)
		return
	}

	token := strings.Split(authHeader[0], " ")[1]
	if token != g.authToken {
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

func (g gui) GetCSRFToken() string {
	return g.CsrfToken
}
