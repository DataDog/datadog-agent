// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package createandfetchimpl implements the creation and access to the auth_token used to communicate between Agent
// processes.
package createandfetchimpl

import (
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/DataDog/datadog-agent/comp/api/authtoken"
)

type secureClient struct {
	http.Client
	authToken string
}

func (at *authToken) GetClient(_ ...authtoken.ClientOption) authtoken.SecureClient {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	return &secureClient{
		Client:    http.Client{Transport: tr},
		authToken: at.token,
	}
}

func (s *secureClient) Do(req *http.Request, opts ...authtoken.RequestOption) (resp []byte, err error) {
	var cb []func()
	onEnded := func(fn func()) {
		cb = append(cb, fn)
	}
	defer func() {
		for _, fn := range cb {
			fn()
		}
	}()

	for _, opt := range opts {
		req = opt(req, onEnded)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.authToken)

	r, err := s.Client.Do(req)

	if err != nil {
		return resp, err
	}
	body, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		return body, err
	}
	if r.StatusCode >= 400 {
		return body, errors.New(string(body))
	}
	return body, nil
}

func (s *secureClient) Get(url string, opts ...authtoken.RequestOption) (resp []byte, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	var cb []func()
	onEnded := func(fn func()) {
		cb = append(cb, fn)
	}
	defer func() {
		for _, fn := range cb {
			fn()
		}
	}()

	for _, opt := range opts {
		req = opt(req, onEnded)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.authToken)
	r, err := s.Client.Do(req)

	if err != nil {
		return resp, err
	}
	body, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		return body, err
	}
	if r.StatusCode >= 400 {
		return body, errors.New(string(body))
	}
	return body, nil
}

func (s *secureClient) Head(url string, opts ...authtoken.RequestOption) (resp []byte, err error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return nil, err
	}

	var cb []func()
	onEnded := func(fn func()) {
		cb = append(cb, fn)
	}
	defer func() {
		for _, fn := range cb {
			fn()
		}
	}()

	for _, opt := range opts {
		req = opt(req, onEnded)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.authToken)
	r, err := s.Client.Do(req)

	if err != nil {
		return resp, err
	}
	body, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		return body, err
	}
	if r.StatusCode >= 400 {
		return body, errors.New(string(body))
	}
	return body, nil
}

func (s *secureClient) Post(url string, contentType string, body io.Reader, opts ...authtoken.RequestOption) (resp []byte, err error) {
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}

	var cb []func()
	onEnded := func(fn func()) {
		cb = append(cb, fn)
	}
	defer func() {
		for _, fn := range cb {
			fn()
		}
	}()

	for _, opt := range opts {
		req = opt(req, onEnded)
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+s.authToken)
	r, err := s.Client.Do(req)

	if err != nil {
		return resp, err
	}
	respBody, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		return respBody, err
	}
	if r.StatusCode >= 400 {
		return respBody, errors.New(string(respBody))
	}
	return respBody, nil
}

func (s *secureClient) PostChunk(url string, contentType string, body io.Reader, onChunk func([]byte), opts ...authtoken.RequestOption) (err error) {
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err
	}

	var cb []func()
	onEnded := func(fn func()) {
		cb = append(cb, fn)
	}
	defer func() {
		for _, fn := range cb {
			fn()
		}
	}()

	for _, opt := range opts {
		req = opt(req, onEnded)
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+s.authToken)
	r, err := s.Client.Do(req)

	if err != nil {
		return err
	}
	defer r.Body.Close()

	var m int
	buf := make([]byte, 4096)
	for {
		m, err = r.Body.Read(buf)
		if m < 0 || err != nil {
			break
		}
		onChunk(buf[:m])
	}

	if r.StatusCode == 200 {
		return nil
	}
	return err
}

func (s *secureClient) PostForm(url string, data url.Values, opts ...authtoken.RequestOption) (resp []byte, err error) {
	return s.Post(url, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()), opts...)
}
