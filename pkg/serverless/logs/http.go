package logs

import (
	"io/ioutil"
	"net/http"
)

// LambdaLogsAPI implements the AWS Lambda Logs API callback
type LambdaLogsAPIServer struct {
	out chan<- []LambdaLogAPIMessage
}

func NewLambdaLogsAPIServer(out chan<- []LambdaLogAPIMessage) LambdaLogsAPIServer {
	return LambdaLogsAPIServer{out}
}

func (l *LambdaLogsAPIServer) Close() {
	close(l.out)
}

// ServeHTTP - see type LambdaLogsCollector comment.
func (c *LambdaLogsAPIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data, _ := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	messages, err := parseLogsAPIPayload(data)
	if err != nil {
		w.WriteHeader(400)
	} else {
		c.out <- messages
		w.WriteHeader(200)
	}
}
