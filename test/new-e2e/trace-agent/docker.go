package traceagent

func dockerRunTraceGen(service string) (string, string) {
	// TODO: use a proper docker-compose definition for tracegen
	run := "docker run -d --network host --rm --name " + service +
		" -e DD_SERVICE=" + service +
		" ghcr.io/datadog/apps-tracegen:main"
	rm := "docker rm -f " + service
	return run, rm
}
