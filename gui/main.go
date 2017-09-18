package main

import (
	"crypto/rsa"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
)

var (
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	tokenName  = "AccessToken"
)

type Message struct {
	Req_type string `json:"req_type"`
	Data     string `json:"data"`
}

// createKeys generates new public and private RSA keys
func createKeys() error {
	_, e := exec.Command("sh", "-c", "openssl genrsa -out keys/app.rsa 2000").Output()
	if e != nil {
		return e
	}

	_, e = exec.Command("sh", "-c", "openssl rsa -in keys/app.rsa -pubout > keys/app.rsa.pub").Output()
	if e != nil {
		return e
	}

	privateB, e := ioutil.ReadFile("keys/app.rsa")
	if e != nil {
		return e
	}

	publicB, e := ioutil.ReadFile("keys/app.rsa.pub")
	if e != nil {
		return e
	}

	privateKey, e = jwt.ParseRSAPrivateKeyFromPEM(privateB)
	if e != nil {
		return e
	}

	publicKey, e = jwt.ParseRSAPublicKeyFromPEM(publicB)
	return e
}

// handlePOST receives POST requests from the front end - only accessible with a valid token
var handleReq = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// Check if the message is from a valid client by fetching & parsing JWT
	tokenCookie, e := r.Cookie(tokenName)
	if e == http.ErrNoCookie {
		w.WriteHeader(http.StatusUnauthorized)
		log.Println("Error: token not present.")
		return
	} else if e != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Cookie parse error: %v", e)
		return
	}

	token, e := jwt.Parse(tokenCookie.Value, func(token *jwt.Token) (interface{}, error) {
		// use private key to sign tokens and public key to verify them
		return publicKey, nil
	})
	switch e.(type) {
	case nil:
		if !token.Valid {
			w.WriteHeader(http.StatusUnauthorized)
			log.Println("Error: invalid token.")
			return
		}
	case *jwt.ValidationError:
		vErr := e.(*jwt.ValidationError)

		if vErr.Errors == jwt.ValidationErrorExpired {
			w.WriteHeader(http.StatusUnauthorized)
			log.Println("Error: token expired.")
			return
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			log.Println("ValidationError error: %+v", vErr.Errors)
			return
		}
	default:
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("Token parse error: %v", e)
		return
	}

	log.Printf("Token validated: %+v\n", token)

	// Decode the data from the request
	body, e := ioutil.ReadAll(r.Body)
	if e != nil {
		log.Println("Error: " + e.Error())
	}
	var m Message
	e = json.Unmarshal(body, &m)
	if e != nil {
		log.Println("ERROR: " + e.Error())
		w.Write([]byte("ERROR: " + e.Error()))
		return
	}

	// Make sure message received was the correct format
	if m.Req_type == "" || m.Data == "" {
		w.Write([]byte("Invalid message received: incorrect format."))
		log.Printf("Invalid message received: incorrect format.")
		return
	}

	/*
			To reply with a proper JSON object:
			payload, _ := json.Marshal(data)
			w.Header().Set("Content-Type", "application/json")
		 	w.Write([]byte(payload))
	*/

	w.Write([]byte("Received message: " + m.Data))
	log.Printf("Data received: %v \n", m)
})

// authenticate generates a JSON web token for authenticated clients and sets it as a cookie
var authenticate = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// Authenticate user using RSA key
	// TODO

	// Create a token
	token := jwt.New(jwt.SigningMethodHS256)

	// Set token claims
	claims := token.Claims.(jwt.MapClaims)
	claims["admin"] = true
	claims["name"] = "Isabelle Sauve"
	claims["exp"] = time.Now().Add(time.Hour * 24).Unix()

	// Sign the token
	tokenString, _ := token.SignedString(privateKey)

	// Write the token to the browser window (as a cookie)
	http.SetCookie(w, &http.Cookie{
		Name:       tokenName,
		Value:      tokenString,
		Path:       "/",
		RawExpires: "0",
	})

	w.WriteHeader(http.StatusOK)
})

func main() {
	// Secure the session by generating new RSA keys
	e := createKeys()
	if e != nil {
		log.Println("Fatal error generating RSA keys: %s", e)
		return
	}

	// Instantiate the gorilla/mux router
	r := mux.NewRouter()

	// Serve the static index page on the default endpoint
	r.Handle("/", http.FileServer(http.Dir("./view/")))

	// Handle requests from clients
	r.Handle("/auth", authenticate).Methods("POST")
	r.Handle("/req", handleReq).Methods("POST")

	http.ListenAndServe(":8080", r)
}
