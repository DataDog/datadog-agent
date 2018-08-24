package procmatch

import "testing"

var avoidOptimization string

var (
	rawIntegs = []Integration{
		{"activeMQ", []string{"activemq"}},
		{"airbrake", []string{"rake airbrake:deploy"}},
		{"apache", []string{"httpd", "apache"}},
		{"bitbucket", []string{"start-bitbucket.sh ", "service atlbitbucket start"}},
		{"bugsnag", []string{"bugsnag-agent", "start bugsnag"}},
		{"cassandra", []string{"java org.apache.cassandra.service.CassandraDaemon"}},
		{"ceph", []string{"ceph-*"}},
		{"consul", []string{"consul agent", "consul_agent", "consul-agent"}},
		{"couchbase", []string{"beam.smp"}},
		{"couchdb", []string{"couchjs"}},
		{"docker", []string{"dockerd", "docker-containerd", "docker run", "docker daemon", " docker-containerd-shim"}},
		{"elasticsearch", []string{"java org.elasticsearch.bootstrap.Elasticsearch"}},
		{"etcd", []string{"etcd"}},
		{"fluentd", []string{"td-agent", "fluentd", "ruby td-agent"}},
		{"gearman", []string{"gearmand", "gearman"}},
		{"gunicorn", []string{"gunicorn: master "}},
		{"haproxy", []string{"haproxy", "haproxy-master"}},
		{"java", []string{"java"}},
		{"kafka", []string{"java kafka.kafka"}},
		{"kong", []string{"kong start"}},
		{"kyototycoon", []string{"ktserver"}},
		{"lighttpd", []string{"lighttpd"}},
		{"marathon", []string{"start --master mesos marathon"}},
		{"memcached", []string{"memcached"}},
		{"mesos", []string{" mesos-agent.sh --master --work_dir=/var/lib/mesos"}},
		{"mongodb", []string{"mongod"}},
		{"mysql", []string{"mysqld"}},
		{"nagios", []string{"service snmpd restart", "systemctl restart snmpd.service"}},
		{"nginx", []string{"nginx: master process"}},
		{"openStack", []string{"stack.sh"}},
		{"pgbouncer", []string{"pgbouncer"}},
		{"php", []string{"php"}},
		{"php-fpm", []string{"php7.0-fpm", "php7.0-fpm start", "service php-fpm", "php7.0-fpm restart", "restart php-fpm", "systemctl restart php-fpm.service", "php7.0-fpm.service"}},
		{"postfix", []string{"postfix start", "sendmail -bd"}},
		{"postgres", []string{"postgres -D", "pg_ctl start -l logfile", "postgres -c 'pg_ctl start -D -l"}},
		{"powerdns_recursor", []string{"pdns_server", "systemctl start pdns@"}},
		{"rabbitmq", []string{"rabbitmq"}},
		{"redis", []string{"redis-server"}},
		{"supervisord", []string{"python supervisord", "supervisord"}},
		{"tomcat", []string{"java tomcat"}},
	}
)

func BenchmarkGraph10(b *testing.B) {
	m, err := NewMatcher(rawIntegs[:10])
	if err != nil {
		return
	}
	benchmarkGraph(b, m)
}

func BenchmarkGraph20(b *testing.B) {
	m, err := NewMatcher(rawIntegs[:20])
	if err != nil {
		return
	}
	benchmarkGraph(b, m)
}

func BenchmarkGraph40(b *testing.B) {
	m, err := NewMatcher(rawIntegs[:40])
	if err != nil {
		return
	}
	benchmarkGraph(b, m)
}

func BenchmarkGraphAll(b *testing.B) {
	m, err := NewMatcher(rawIntegs)
	if err != nil {
		return
	}
	benchmarkGraph(b, m)
}

func benchmarkGraph(b *testing.B, m Matcher) {
	test := "myprogram -Xmx4000m -Xms4000m -XX:ReservedCodeCacheSize=256m -port 9999 kafka.Kafka"

	var r string
	for n := 0; n < b.N; n++ {
		r = m.Match(test)
	}
	avoidOptimization = r
}
