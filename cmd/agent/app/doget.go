package app

import (
	"io"
	"io/ioutil"
	"net/http"
)

func doGet(c *http.Client, url string) (body []byte, e error) {
	r, e := c.Get(url)
	if e != nil {
		return body, e
	}
	body, e = ioutil.ReadAll(r.Body)
	r.Body.Close()
	if e != nil {
		return body, e
	}
	return body, nil

}

func doPost(c *http.Client, url string, contentType string, body io.Reader) (resp []byte, e error) {
	r, e := c.Post(url, contentType, body)
	if e != nil {
		return resp, e
	}
	resp, e = ioutil.ReadAll(r.Body)
	r.Body.Close()
	if e != nil {
		return resp, e
	}
	return resp, nil

}
