// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build docker

package docker

import (
	"bytes"
	"encoding/binary"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	iParser "github.com/DataDog/datadog-agent/pkg/logs/parser"
	"regexp"
)

var (
	delimiter = []byte{' '}
	// 2019-05-29T09:26:32.155255473Z
	timestampMatcher = regexp.MustCompile("\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}\\.\\d{9}Z")
	// Escaped CRLF, used for determine empty messages
	windowsEOFMark     = []byte{'\\', 'r', '\\', 'n'}
	carriageReturnMark = []byte{'\\', 'r'}
	lineFeedMark       = []byte{'\\', 'n'}
)

const (
	// [8ByteHeaderTimestamp Message]
	numOfComponents    = 2

	// message indices
	indexOfHeader      = 0
	indexOfContent     = 1

	// 8Byte header indices
	indexOfTimestamp   = 8
	indexOfContentSize = 4
	indexOfStatus      = 0
)

// Convertor specific for docker logs read from socket.
type Convertor struct {
	iParser.Convertor
}

// Convert converts the input message to Line, if it's partial msg, defaultPrefix will be
// used as prefix.
func (c *Convertor) Convert(msg []byte, defaultPrefix iParser.Prefix) *iParser.Line {
	components := bytes.SplitN(msg, delimiter, numOfComponents)
	if !c.validateHeader(components[indexOfHeader]) {
		// take this msg as partial log splitted by upstream.
		return &iParser.Line{
			Prefix:  defaultPrefix,
			Content: msg,
			Size:    len(msg),
		}
	}

	if c.emptyContent(components) {
		return nil
	}

	status, timestamp, contentLen := c.parseHeader(components[indexOfHeader])
	if contentLen <= 0 && status == "" { // take it as tty, which means msg does not contain 8 bytes docker header.
		return &iParser.Line{
			Prefix: iParser.Prefix{
				Status:    message.StatusInfo,
				Timestamp: timestamp,
			},
			Content: components[indexOfContent],
			Size:    len(components[indexOfContent]),
		}
	}
	content := c.parseContent(msg)
	return &iParser.Line{
		Prefix: iParser.Prefix{
			Status:    status,
			Timestamp: timestamp,
		},
		Content: content,
		Size:    len(content),
	}
}

func (c *Convertor) emptyContent(components [][]byte) bool {
	// consider as empty content when:
	// there is no content byte array at all, or
	// content is considered to be empty.
	return len(components) < numOfComponents || c.isEmptyMsg(components[indexOfContent])
}

// parseContent removes the 8 byte header, timestamp, and space that occurs between 16KB section of a log.
// If a docker log is greater than 16KB, each 16KB partial section will have a header, timestamp, and space in front of it.
// For example, a message that is 35KB will be of the form: `H M1H M2H M3` where "H" is what pre-pends each 16 KB section.
// This function removes the "H " between two partial messages sections while leaving the very first "H ".
// Input:
//   H M1H M2H M3
// Output:
//   H M1M2M3
func (c *Convertor) parseContent(msg []byte) []byte {
	var result bytes.Buffer
	for len(msg) > 0 {
		components := bytes.SplitN(msg, delimiter, numOfComponents)
		_, _, length := c.parseHeader(components[indexOfHeader])
		result.Write(components[indexOfContent][:length])
		msg = components[indexOfContent][length:]
	}
	return result.Bytes()
}

func bytesToUint32(bytes []byte) int {
	return int(binary.BigEndian.Uint32(bytes))
}

func (c *Convertor) parseHeader(header []byte) (string, string, int) {
	var status string
	var timestamp string
	var contentLen int
	// rely on the first byte of header to know whether this header contains leading
	// 8 Bytes docker header or not.
	// If header contains leading 8 bytes metadata, the first byte should be the status
	// with value 1(stdout) or 2(stderr)
	// otherwise, treat this header as tty log, in which case, the header is just
	// a timestamp.
	switch header[indexOfStatus] {
	case 1:
		status = message.StatusInfo
		timestamp = string(header[indexOfTimestamp:])
		contentLen = bytesToUint32(header[indexOfContentSize:indexOfTimestamp])
	case 2:
		status = message.StatusError
		timestamp = string(header[indexOfTimestamp:])
		contentLen = bytesToUint32(header[indexOfContentSize:indexOfTimestamp])
	default:
		timestamp = string(header)
	}

	return status, timestamp, contentLen
}

// validate validates the docker header, which should be in this format:
// [severity, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4], where
// serverity is 1 as stdout, or 2 as stderr
// header[1:4] is with fixed value [0, 0, 0]
// SIZE1, SIZE2, SIZE3, and SIZE4 are four bytes of uint32 encoded as big endian.
// This is the size of OUTPUT.
// the last part is timstamp in this format: 2019-05-29T09:26:32.155255473Z
// the full header is: �2019-05-29T09:26:32.155255473Z
// In case of docker exec with tty, 8 bytes header is absent. In this case the full
// header becomes: 2019-05-29T09:26:32.155255473Z, which should also be valid.
func (c *Convertor) validateHeader(header []byte) bool {
	hasStdOutPrefix := bytes.HasPrefix(header, headerStdoutPrefix)
	hasStdErrPrefix := bytes.HasPrefix(header, headerStderrPrefix)
	return (!hasStdOutPrefix && !hasStdErrPrefix && timestampMatcher.MatchString(string(header))) || // tty
		((hasStdErrPrefix || hasStdOutPrefix) && timestampMatcher.MatchString(string(header[indexOfTimestamp:])))
}

// isNewLineOnly checks if the content is in the form of escaped new line or empty.
// i.e. \\n or \\r or \\r\\n
func (c *Convertor) isEmptyMsg(content []byte) bool {
	return len(content) <= 0 ||
		bytes.Equal(content, carriageReturnMark) ||
		bytes.Equal(content, lineFeedMark) ||
		bytes.Equal(content, windowsEOFMark)
}
