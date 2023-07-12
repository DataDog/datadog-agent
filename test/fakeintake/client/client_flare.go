// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"archive/zip"
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"mime/multipart"
	"strings"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// This file contains the logic for parsing the flare content returned by the Fake Intake and transform it into
// a manageable Flare structure.
//
// Datadog Agent is sending the flare using MIME Multipart media type as defined in RFC 2046. The multipart body contains the following:
// * `email`: email provided when creating the flare.
// * `flare_file`: the zip file created by the Agent
// * `agent_version`: the version of the Agent which created the flare.
// * `hostname`: hostname of the host on which the flare was created.
// TODO: more part might exist like `case_id` or `source` which has been added recently
//
// This body is following a format similar to the one below:
//
/*
	--0cf50bf933f0ddecdd8cbfada84c4d6fa4cb226fd5893493f51fd01dfea5
	Content-Disposition: form-data; name="email"
	Content-Type: text/plain

	test@mail.com
	--0cf50bf933f0ddecdd8cbfada84c4d6fa4cb226fd5893493f51fd01dfea5
	Content-Disposition: form-data; name="flare_file"; filename="flare.zip"
	Content-Type: application/octet-stream

	<raw zip file content>
	--0cf50bf933f0ddecdd8cbfada84c4d6fa4cb226fd5893493f51fd01dfea5
	Content-Disposition: form-data; name="agent_version"
	Content-Type: text/plain

	7.45.0
	--0cf50bf933f0ddecdd8cbfada84c4d6fa4cb226fd5893493f51fd01dfea5
	Content-Disposition: form-data; name="hostname"
	Content-Type: text/plain

	hostname123
	--0cf50bf933f0ddecdd8cbfada84c4d6fa4cb226fd5893493f51fd01dfea5--
*/
// On this example the pattern '0cf50bf933f0ddecdd8cbfada84c4d6fa4cb226fd5893493f51fd01dfea5' is the second argument `boundary` and
// each content between two boundaries is fetched with `mimeReader.NextPart()` in the function below.
// The boundary is provided by the initial flare request from the Agent in the `Content-Type` header,
// 	for example: Content-Type: multipart/form-data; boundary=0cf50bf933f0ddecdd8cbfada84c4d6fa4cb226fd5893493f51fd01dfea5
// and the `Content-Type` value is obtained from the Fake Intake via the "encoding" key
//
// The flare zip content is as well parsed and then slightly transformed into a mapping between filenames and File to make querying
// simpler (e.g. verify that `agent.log` exists and has the right permissions)
//
// The resulting output is a Flare struct that provides an API to verify assertions on the flare.

// Flare contains all the information sent by the Datadog Agent when using the Flare command
// zipFileMap is a mapping between filenames and *zip.File obtained from zip.Reader struct.
type Flare struct {
	email        string
	zipFileMap   map[string]*zip.File
	agentVersion string
	hostname     string
}

// getFlare queries the Fake Intake to fetch flare that were sent by a Datadog Agent and returns a Flare struct
// TODO: handle multiple flares
func (c *Client) getFlare() (Flare, error) {
	payloads, err := c.getFakePayloads("/support/flare")
	if err != nil {
		return Flare{}, err
	}

	// TODO: create a flare aggregator and populate flare after parsing it + return an error
	return parseRawFlare(payloads[0])
}

// decodeRawFlare parses the flare payload sent by the Fake Intake into a manageable Flare struct
// For that it parses the multipart data from the flare request and then parses the flare zip raw content.
func parseRawFlare(flarePayload api.Payload) (Flare, error) {
	// flarePayload.Encoding contains the value of Content-Type header from the flare request
	boundary := parseBoundaryFromContentTypeHeader(flarePayload.Encoding)
	parsedFlareData, err := parseFlareMultipartData(string(flarePayload.Data), boundary)
	if err != nil {
		return Flare{}, err
	}

	// flare_file is the only part that needs a special parsing as it's the flare zip content.
	zipFileMap, err := parseRawZIP(parsedFlareData["flare_file"])
	if err != nil {
		return Flare{}, err
	}

	return Flare{
		email:        string(parsedFlareData["email"]),
		zipFileMap:   zipFileMap,
		agentVersion: string(parsedFlareData["agent_version"]),
		hostname:     string(parsedFlareData["hostname"]),
	}, nil
}

// parseBoundaryFromContentTypeHeader parses the value of the Content-Type header sent by the Agent when sending the flare
// and returns the boundary linked to the multipart data.
//
// Example:
//
//	Input: "multipart/form-data; boundary=0cf50bf933f0ddecdd8cbfada84c4d6fa4cb226fd5893493f51fd01dfea5"
//	Output: "0cf50bf933f0ddecdd8cbfada84c4d6fa4cb226fd5893493f51fd01dfea5"
func parseBoundaryFromContentTypeHeader(contentTypeHeader string) string {
	var encoding string

	mediaType, params, err := mime.ParseMediaType(contentTypeHeader)
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		log.Printf("Could not parse Content-Type header from flare request")
		encoding = ""
	} else {
		encoding = params["boundary"]
	}

	return encoding
}

// parseFlareMultipartData is responsible for parsing the raw multipart data and transform it into a mapping between field name and content.
// For the above example the returned value would be equivalent to the following (except that values are []byte)
//
//	{
//			"email": "test@mail.com",
//			"flare_file": <Zip raw content>,
//			"agent_version": "7.45.0",
//			"hostname": "hostname123"
//	}
func parseFlareMultipartData(data string, boundary string) (map[string][]byte, error) {
	var multipartNameToContent = make(map[string][]byte)

	mimeReader := multipart.NewReader(strings.NewReader(data), boundary)

	for {
		part, err := mimeReader.NextPart()

		if err == io.EOF {
			break
		}

		if err != nil {
			return multipartNameToContent, err
		}

		body, err := ioutil.ReadAll(part)
		if err != nil {
			return multipartNameToContent, err
		}
		multipartNameToContent[part.FormName()] = body
	}

	return multipartNameToContent, nil
}

// decodeRawZip takes the raw content of a zip file, reads it and then creates a mapping between filenames and *zip.File.
// We create this mapping (instead of just using []*zip.File provided by zip.Reader) to easily query a specific file and verify assertions on it.
func parseRawZIP(rawContent []byte) (map[string]*zip.File, error) {
	var zipFileMap = make(map[string]*zip.File)

	buffer := bytes.NewReader(rawContent)
	reader, err := zip.NewReader(buffer, int64(len(rawContent)))
	if err != nil {
		return zipFileMap, err
	}

	for _, file := range reader.File {
		zipFileMap[file.Name] = file
	}

	return zipFileMap, nil
}
