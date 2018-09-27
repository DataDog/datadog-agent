package procmatch

import (
	"testing"
)

var testMatcher Matcher

func init() {
	testMatcher, _ = NewDefault()
}

func assertIntegration(t *testing.T, e string, i string) {
	if e != i {
		t.Errorf("%s failed, wrong integration name, expected '%s' but got '%s'", t.Name(), e, i)
	}
}

func TestMatchIntegration(t *testing.T) {
	cases := []struct {
		cmdline     string
		integration string
	}{
		{"java org.elasticsearch.bootstrap.Elasticsearch ‑Xms28000m ‑Xmx28000m ‑XX:+UseCompressedOops ‑Djna.tmpdir=/tmp/elasticsearch/jna ‑XX:+UseConcMarkSweepGC ‑XX:CMSInitiatingOccupancyFraction=75 ‑XX:+UseCMSInitiatingOccupancyOnly ‑XX:+DisableExplicitGC ‑XX:+AlwaysPreTouch ‑server ‑Xss1m ‑Djava.awt.headless=true ‑Dfile.encoding=UTF-8 ‑Djna.nosys=true ‑Djdk.io.permissionsUseCanonicalPath=true ‑Dio.netty.noUnsafe=true ‑Dio.netty.noKeySetOptimization=true ‑Dio.netty.recycler.maxCapacityPerThread=0 ‑Dlog4j.shutdownHookEnabled=false ‑Dlog4j2.disable.jmx=true ‑Dlog4j.skipJansi=true ‑Des.path.home=/usr/share/elasticsearch ‑Des.path.conf=/config ‑cp /usr/share/elasticsearch/lib/* ‑p /var/run/elasticsearch.pid ‑Epath.logs=/logs ‑Epath.data=/data ",
			"elastic"},
		{"gunicorn: master [mcnulty]",
			"gunicorn"},
		{"java kafka.Kafka /usr/local/kafka/config/server.properties ‑Xmx4G ‑Xms4G ‑server ‑XX:+UseCompressedOops ‑XX:PermSize=48m ‑XX:MaxPermSize=48m ‑XX:+UseG1GC ‑XX:MaxGCPauseMillis=20 ‑XX:InitiatingHeapOccupancyPercent=35 ‑Djava.awt.headless=true ‑Xloggc:/mnt/log/kafka/kafkaServer-gc.log ‑verbose:gc ‑XX:+PrintGCDetails ‑XX:+PrintGCDateStamps ‑XX:+PrintGCTimeStamps ‑Dcom.sun.management.jmxremote ‑Dcom.sun.management.jmxremote.authenticate=false ‑Dcom.sun.management.jmxremote.ssl=false ‑Dcom.sun.management.jmxremote.port=9999",
			"kafka"},
		{"haproxy ‑p /run/haproxy.pid ‑db ‑f /usr/local/etc/haproxy/haproxy.cfg ‑Ds",
			"haproxy"},
		{"mongod ‑-config /config/mongodb.conf",
			"mongo"},
		{"java -Xmx4000m -Xms4000m -XX:ReservedCodeCacheSize=256m -port 9999 kafka.Kafka",
			"kafka"},
		{"java -Xmx4000m -Xms4000m -XX:ReservedCodeCacheSize=256m -port 9999 kafka.Kafka",
			"kafka"},
		{"/usr/local/bin/consul agent -config-dir /etc/consul.d",
			"consul"},
		{"/usr/bin/python /usr/local/bin/supervisord -c /etc/supervisord.conf",
			"supervisord"},
		{"/usr/sbin/pgbouncer -d /etc/pgbouncer/pgbouncer.ini",
			"pgbouncer"},
	}

	for _, c := range cases {
		matched := testMatcher.Match(c.cmdline)
		assertIntegration(t, c.integration, matched.Name)
	}
}

func TestOverlappingSignatures(t *testing.T) {
	cases := []struct {
		cmdline     string
		integration string
	}{
		{"java org.elasticsearch.bootstrap.Elasticsearch -p=mypath",
			"elastic"},
		{"java org.elasticsearch.bootstrap.Elasticsearch",
			"elastic"},
		{"java ***** kafka.kafka",
			"kafka"},
	}

	for _, c := range cases {
		matched := testMatcher.Match(c.cmdline)
		assertIntegration(t, c.integration, matched.Name)
	}
}

// Test that the signatures defined in the default catalog are matches the integrations of the catalog
func TestDefaultCatalogOnGraph(t *testing.T) {
	for _, integration := range DefaultCatalog {
		for _, cmd := range integration.Signatures {
			matched := testMatcher.Match(cmd)
			assertIntegration(t, integration.Name, matched.Name)
		}
	}
}
