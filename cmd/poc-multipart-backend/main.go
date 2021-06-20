package main

import (
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
)

type wrapper struct {
	wrapped           io.ReadCloser
	contentsBytesRead int
	contents          []byte
	boundary          string
}

func (w *wrapper) Read(p []byte) (n int, err error) {
	off := 0
	if w.contentsBytesRead < len(w.contents) {
		max := len(w.contents)
		if max > len(p) {
			max = len(p)
		}

		copy(p[0:max], w.contents[w.contentsBytesRead:w.contentsBytesRead+max])
		w.contentsBytesRead += max

		off = max
	}

	if off >= len(p) {
		return off, nil
	}

	read, err := w.wrapped.Read(p[off:])
	n = off + read

	return n, err
}

func (w *wrapper) Close() error {
	// return w.wrapped.Close()
	return nil
}

func wrap(body io.ReadCloser, boundary string) *wrapper {
	contents := fmt.Sprintf("--%s\r\nContent-Disposition: form-data; name=\"injected\"\n\nvalue3\r\n", boundary)

	return &wrapper{
		wrapped:           body,
		boundary:          boundary,
		contents:          []byte(contents),
		contentsBytesRead: 0,
	}
}

func handleMultipart(w http.ResponseWriter, req *http.Request) {
	contentType := req.Header.Get("Content-Type")
	if contentType == "" {
		return
	}
	mimeType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		fmt.Fprintf(w, "ERROR: %+v\n", err)
		return
	}

	if mimeType != "multipart/form-data" {
		return
	}
	boundary := params["boundary"]

	if boundary == "" {
		fmt.Fprintln(w, "ERROR: no boundary")
	}
	fmt.Fprintln(w, "------------")

	mr := multipart.NewReader(wrap(req.Body, boundary), boundary)
	// mr := multipart.NewReader(req.Body, boundary)

	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			return
		}
		if err != nil {
			log.Fatal(err)
		}
		slurp, err := io.ReadAll(p)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Fprintf(w, "Part %q: %q\n", p.FormName(), slurp)
	}
}

func reflect(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "%s %s %s\n", req.Method, req.URL, req.Proto)
	fmt.Fprintf(w, "Host: %s\n", req.Host)
	for name, headers := range req.Header {
		for _, h := range headers {
			fmt.Fprintf(w, "%v: %v\n", name, h)
		}
	}
	// io.Copy(w, wrap(req.Body, "------------------------e1742021ddf5a817"))
	// io.Copy(w, req.Body)
	handleMultipart(w, req)
}

func main() {
	http.HandleFunc("/", reflect)
	http.ListenAndServe("[::1]:8090", nil)
}
