package procmatch

import "testing"

var avoidOptimization string

var (
	rawIntegs = []Integration{
		{"ActiveMQ", []string{"activemq"}},
		{"Airbrake", []string{"rake airbrake:deploy"}},
		{"Apache", []string{"httpd", "apache"}},
		{"BitBucket", []string{"start-bitbucket.sh ", "service atlbitbucket start"}},
		{"Bugsnag", []string{"bugsnag-agent", "start bugsnag"}},
		{"Cassandra", []string{"java org.apache.cassandra.service.CassandraDaemon"}},
		{"Ceph", []string{"ceph-*"}},
		{"Consul", []string{"consul agent", "consul_agent", "consul-agent"}},
		{"Couchbase", []string{"beam.smp"}},
		{"CouchDB", []string{"couchjs"}},
		{"Docker", []string{"dockerd", "docker-containerd", "docker run", "docker daemon", " docker-containerd-shim"}},
		{"Elasticsearch", []string{"java org.elasticsearch.bootstrap.Elasticsearch"}},
		{"Etcd", []string{"etcd"}},
		{"fluentd", []string{"td-agent", "fluentd", "ruby td-agent"}},
		{"Gearman", []string{"gearmand", "gearman"}},
		{"Gunicorn", []string{"gunicorn: master "}},
		{"HAProxy", []string{"haproxy", "haproxy-master"}},
		{"Java", []string{"java"}},
		{"Kafka", []string{"java kafka.kafka"}},
		{"Kong", []string{"kong start"}},
		{"Kyototycoon", []string{"ktserver"}},
		{"Lighttpd", []string{"lighttpd"}},
		{"Marathon", []string{"start --master mesos marathon"}},
		{"Memcached", []string{"memcached"}},
		{"Mesos", []string{" mesos-agent.sh --master --work_dir=/var/lib/mesos"}},
		{"Mongodb", []string{"mongod"}},
		{"Mysql", []string{"mysqld"}},
		{"Nagios", []string{"service snmpd restart", "systemctl restart snmpd.service"}},
		{"Nginx", []string{"nginx: master process"}},
		{"OpenStack", []string{"stack.sh"}},
		{"Pgbouncer", []string{"pgbouncer"}},
		{"PHP", []string{"php"}},
		{"PHP-FPM", []string{"php7.0-fpm", "php7.0-fpm start", "service php-fpm", "php7.0-fpm restart", "restart php-fpm", "systemctl restart php-fpm.service", "php7.0-fpm.service"}},
		{"Postfix", []string{"postfix start", "sendmail -bd"}},
		{"Postgres", []string{"postgres -D", "pg_ctl start -l logfile", "postgres -c 'pg_ctl start -D -l"}},
		{"PowerDNS Recursor", []string{"pdns_server", "systemctl start pdns@"}},
		{"RabbitMQ", []string{"rabbitmq"}},
		{"Redis", []string{"redis-server"}},
		{"Supervisord", []string{"python supervisord", "supervisord"}},
		{"Tomcat", []string{"java tomcat"}},
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
