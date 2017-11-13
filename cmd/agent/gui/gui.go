package gui

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"net"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	log "github.com/cihub/seelog"
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
	"github.com/urfave/negroni"
)

var (
	listener   net.Listener
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	csrfToken  string
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

// StartGUI creates the router, starts the HTTP server and opens the GUI in a browser
func StartGUI(port string) error {
	// Instantiate the gorilla/mux router
	router := mux.NewRouter()

	// Serve the only public file at the authentication endpoint
	router.HandleFunc("/authenticate", generateAuthEndpoint)

	// Serve the (secured) index page on the default endpoint
	router.Handle("/", authorizeAccess(http.FileServer(http.Dir(filepath.Join(common.GetViewsPath(), "private")))))

	// Mount our (secured) filesystem at the view/{path} route
	router.PathPrefix("/view/").Handler(http.StripPrefix("/view/", authorizeAccess(http.FileServer(http.Dir(filepath.Join(common.GetViewsPath(), "private"))))))

	// Set up handlers for the API
	agentRouter := mux.NewRouter().PathPrefix("/agent").Subrouter().StrictSlash(true)
	agentHandler(agentRouter)
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
	go http.Serve(listener, router)

	// Generate a pair of RSA keys
	privateKey, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		return fmt.Errorf("error generating RSA key: " + e.Error())
	}
	publicKey = &privateKey.PublicKey

	// Create a JWT signed with the private key
	JWT := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"admin": true,
		"name":  "Datadog Agent Manager",
	})
	jwtString, e := JWT.SignedString(privateKey)
	if e != nil {
		return fmt.Errorf("error creating JWT: " + e.Error())
	}

	// Create a CSRF token
	key := make([]byte, 32)
	_, e = rand.Read(key)
	if e != nil {
		return fmt.Errorf("error creating CSRF token: " + e.Error())
	}
	csrfToken = hex.EncodeToString(key)

	// Open the GUI in a browser, passing the authorization tokens as parameters
	e = open("http://127.0.0.1:" + port + "/authenticate?jwt=" + jwtString + ";csrf=" + csrfToken)
	if e != nil {
		return fmt.Errorf("error opening GUI: " + e.Error())
	}

	log.Infof("GUI opened at 127.0.0.1:" + port)
	return nil
}

func generateAuthEndpoint(w http.ResponseWriter, r *http.Request) {
	t, e := template.New("auth.tmpl").ParseFiles(filepath.Join(common.GetViewsPath(), "templates/auth.tmpl"))
	if e != nil {
		http.Error(w, e.Error(), http.StatusInternalServerError)
		return
	}

	e = t.Execute(w, map[string]interface{}{"csrf": csrfToken})
	if e != nil {
		http.Error(w, e.Error(), http.StatusInternalServerError)
		return
	}
}

// Middleware which blocks access to secured files from unauthorized clients
func authorizeAccess(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Disable caching
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

		cookie, _ := r.Cookie("jwt")
		if cookie == nil {
			w.WriteHeader(http.StatusUnauthorized)
			http.Error(w, "no authorization token", 401)
			return
		}

		e := verifyJWT(w, cookie.Value)
		if e != nil {
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

	e := verifyJWT(w, strings.Split(authHeader[0], " ")[1])
	if e != nil {
		return
	}

	next(w, r)
}

func verifyJWT(w http.ResponseWriter, tokenString string) error {
	// Use public key to verify the token
	token, e := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return publicKey, nil
	})

	if e != nil {
		w.WriteHeader(http.StatusInternalServerError)
		http.Error(w, "token validation error: "+e.Error(), 500)
	} else if !token.Valid {
		w.WriteHeader(http.StatusUnauthorized)
		http.Error(w, "invalid authorization token", 401)
		e = fmt.Errorf("invalid authorization token")
	}

	return e
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
