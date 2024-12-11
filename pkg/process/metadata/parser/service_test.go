// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parser

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func TestExtractServiceMetadata(t *testing.T) {
	tests := []struct {
		name                 string
		cmdline              []string
		useImprovedAlgorithm bool
		expectedServiceTag   string
	}{
		{
			name:               "empty",
			cmdline:            []string{},
			expectedServiceTag: "",
		},
		{
			name:               "blank",
			cmdline:            []string{""},
			expectedServiceTag: "",
		},
		{
			name: "single arg executable",
			cmdline: []string{
				"./my-server.sh",
			},
			expectedServiceTag: "process_context:my-server",
		},
		{
			name: "single arg executable with envs",
			cmdline: []string{
				"SOME=THING", "./my-server.sh",
			},
			expectedServiceTag: "process_context:my-server",
		},
		{
			name: "sudo",
			cmdline: []string{
				"sudo", "-E", "-u", "dog", "/usr/local/bin/myApp", "-items=0,1,2,3", "-foo=bar",
			},
			expectedServiceTag: "process_context:myApp",
		},
		{
			name: "python flask argument",
			cmdline: []string{
				"/opt/python/2.7.11/bin/python2.7", "flask", "run", "--host=0.0.0.0",
			},
			expectedServiceTag: "process_context:flask",
		},
		{
			name: "python - flask argument in path",
			cmdline: []string{
				"/opt/python/2.7.11/bin/python2.7", "/opt/dogweb/bin/flask", "run", "--host=0.0.0.0", "--without-threads",
			},
			expectedServiceTag: "process_context:flask",
		},
		{
			name: "python flask in single argument",
			cmdline: []string{
				"/opt/python/2.7.11/bin/python2.7 flask run --host=0.0.0.0",
			},
			expectedServiceTag: "process_context:flask",
		},
		{
			name: "python flask in single argument with envs",
			cmdline: []string{
				"ENV=VALUE /opt/python/2.7.11/bin/python2.7 flask run --host=0.0.0.0",
			},
			expectedServiceTag: "process_context:flask",
		},
		{
			name: "python flask in single argument with DD_SERVICE",
			cmdline: []string{
				"DD_SERVICE=svc /opt/python/2.7.11/bin/python2.7 flask run --host=0.0.0.0",
			},
			expectedServiceTag: "process_context:svc",
		},
		{
			name: "python - module hello",
			cmdline: []string{
				"python3", "-m", "hello",
			},
			expectedServiceTag: "process_context:hello",
		},
		{
			name: "python - module hello with unrelated env",
			cmdline: []string{
				"SOME=THING", "python3", "-m", "hello",
			},
			expectedServiceTag: "process_context:hello",
		},
		{
			name: "python - module hello with DD_SERVICE",
			cmdline: []string{
				"SOME=THING", "DD_SERVICE=myservice", "python3", "-m", "hello",
			},
			expectedServiceTag: "process_context:myservice",
		},
		{
			name: "python - zip file",
			cmdline: []string{
				"python3", "./hello.zip",
			},
			expectedServiceTag: "process_context:hello.zip",
		},
		{
			name: "python - zip file - improved algorithm",
			cmdline: []string{
				"python3", "./hello.zip",
			},
			useImprovedAlgorithm: true,
			expectedServiceTag:   "process_context:hello",
		},
		{
			name: "python .py",
			cmdline: []string{
				"python3", "hello.py",
			},
			expectedServiceTag: "process_context:hello.py",
		},
		{
			name: "ruby - td-agent",
			cmdline: []string{
				"ruby", "/usr/sbin/td-agent", "--log", "/var/log/td-agent/td-agent.log", "--daemon", "/var/run/td-agent/td-agent.pid",
			},
			expectedServiceTag: "process_context:td-agent",
		},
		{
			name: "java with envs",
			cmdline: []string{
				"DD_TAGS=a:b,c:d,service:mytag,e:f", "java", "-Xmx4000m", "-Xms4000m", "-XX:ReservedCodeCacheSize=256m", "-jar", "/opt/sheepdog/bin/myservice.jar",
			},
			expectedServiceTag: "process_context:mytag",
		},
		{
			name: "java using the -jar flag to define the service",
			cmdline: []string{
				"java", "-Xmx4000m", "-Xms4000m", "-XX:ReservedCodeCacheSize=256m", "-jar", "/opt/sheepdog/bin/myservice.jar",
			},
			expectedServiceTag: "process_context:myservice",
		},
		{
			name: "java class name as service",
			cmdline: []string{
				"java", "-Xmx4000m", "-Xms4000m", "-XX:ReservedCodeCacheSize=256m", "com.datadog.example.HelloWorld",
			},
			expectedServiceTag: "process_context:HelloWorld",
		},
		{
			name: "java kafka",
			cmdline: []string{
				"java", "-Xmx4000m", "-Xms4000m", "-XX:ReservedCodeCacheSize=256m", "kafka.Kafka",
			},
			expectedServiceTag: "process_context:Kafka",
		},
		{
			name: "java parsing for org.apache projects with cassandra as the service",
			cmdline: []string{
				"/usr/bin/java", "-Xloggc:/usr/share/cassandra/logs/gc.log", "-ea", "-XX:+HeapDumpOnOutOfMemoryError", "-Xss256k", "-Dlogback.configurationFile=logback.xml",
				"-Dcassandra.logdir=/var/log/cassandra", "-Dcassandra.storagedir=/data/cassandra",
				"-cp", "/etc/cassandra:/usr/share/cassandra/lib/HdrHistogram-2.1.9.jar:/usr/share/cassandra/lib/cassandra-driver-core-3.0.1-shaded.jar",
				"org.apache.cassandra.service.CassandraDaemon",
			},
			expectedServiceTag: "process_context:cassandra",
		},
		{
			name: "java with -m flag",
			cmdline: []string{
				"java", "-Des.networkaddress.cache.ttl=60", "-Des.networkaddress.cache.negative.ttl=10", "-Djava.security.manager=allow", "-XX:+AlwaysPreTouch",
				"-Xss1m", "-Djava.awt.headless=true", "-Dfile.encoding=UTF-8", "-Djna.nosys=true", "-XX:-OmitStackTraceInFastThrow", "-Dio.netty.noUnsafe=true",
				"-Dio.netty.noKeySetOptimization=true", "-Dio.netty.recycler.maxCapacityPerThread=0", "-Dlog4j.shutdownHookEnabled=false", "-Dlog4j2.disable.jmx=true",
				"-Dlog4j2.formatMsgNoLookups=true", "-Djava.locale.providers=SPI,COMPAT", "--add-opens=java.base/java.io=org.elasticsearch.preallocate",
				"-XX:+UseG1GC", "-Djava.io.tmpdir=/tmp/elasticsearch-11638915669270544049", "-XX:+HeapDumpOnOutOfMemoryError", "-XX:+ExitOnOutOfMemoryError",
				"-XX:HeapDumpPath=data", "-XX:ErrorFile=logs/hs_err_pid%p.log", "-Xlog:gc*,gc+age=trace,safepoint:file=logs/gc.log:utctime,level,pid,tags:filecount=32,filesize=64m",
				"-Des.cgroups.hierarchy.override=/", "-XX:ActiveProcessorCount=1", "-Djava.net.preferIPv4Stack=true", "-XX:-HeapDumpOnOutOfMemoryError", "-Xms786m", "-Xmx786m",
				"-XX:MaxDirectMemorySize=412090368", "-XX:G1HeapRegionSize=4m", "-XX:InitiatingHeapOccupancyPercent=30", "-XX:G1ReservePercent=15", "-Des.distribution.type=tar",
				"--module-path", "/usr/share/elasticsearch/lib", "--add-modules=jdk.net", "--add-modules=org.elasticsearch.preallocate", "-m",
				"org.elasticsearch.server/org.elasticsearch.bootstrap.Elasticsearch",
			},
			expectedServiceTag: "process_context:Elasticsearch",
		},
		{
			name: "java with --module flag",
			cmdline: []string{
				"java", "-Des.networkaddress.cache.ttl=60", "-Des.networkaddress.cache.negative.ttl=10", "-Djava.security.manager=allow", "-XX:+AlwaysPreTouch",
				"-Xss1m", "-Djava.awt.headless=true", "-Dfile.encoding=UTF-8", "-Djna.nosys=true", "-XX:-OmitStackTraceInFastThrow", "-Dio.netty.noUnsafe=true",
				"-Dio.netty.noKeySetOptimization=true", "-Dio.netty.recycler.maxCapacityPerThread=0", "-Dlog4j.shutdownHookEnabled=false", "-Dlog4j2.disable.jmx=true",
				"-Dlog4j2.formatMsgNoLookups=true", "-Djava.locale.providers=SPI,COMPAT", "--add-opens=java.base/java.io=org.elasticsearch.preallocate",
				"-XX:+UseG1GC", "-Djava.io.tmpdir=/tmp/elasticsearch-11638915669270544049", "-XX:+HeapDumpOnOutOfMemoryError", "-XX:+ExitOnOutOfMemoryError",
				"-XX:HeapDumpPath=data", "-XX:ErrorFile=logs/hs_err_pid%p.log", "-Xlog:gc*,gc+age=trace,safepoint:file=logs/gc.log:utctime,level,pid,tags:filecount=32,filesize=64m",
				"-Des.cgroups.hierarchy.override=/", "-XX:ActiveProcessorCount=1", "-Djava.net.preferIPv4Stack=true", "-XX:-HeapDumpOnOutOfMemoryError", "-Xms786m", "-Xmx786m",
				"-XX:MaxDirectMemorySize=412090368", "-XX:G1HeapRegionSize=4m", "-XX:InitiatingHeapOccupancyPercent=30", "-XX:G1ReservePercent=15", "-Des.distribution.type=tar",
				"--module-path", "/usr/share/elasticsearch/lib", "--add-modules=jdk.net", "--add-modules=org.elasticsearch.preallocate", "--module",
				"org.elasticsearch.server/org.elasticsearch.bootstrap.Elasticsearch",
			},
			expectedServiceTag: "process_context:Elasticsearch",
		},
		{
			name: "java with --module flag without main class",
			cmdline: []string{
				"java", "-Des.networkaddress.cache.ttl=60", "-Des.networkaddress.cache.negative.ttl=10", "-Djava.security.manager=allow", "-XX:+AlwaysPreTouch",
				"--module-path", "/usr/share/elasticsearch/lib", "--add-modules=jdk.net", "--add-modules=org.elasticsearch.preallocate", "--module",
				"org.elasticsearch.server",
			},
			expectedServiceTag: "process_context:server",
		},
		{
			name: "java space in java executable path",
			cmdline: []string{
				"/home/dd/my java dir/java", "com.dog.cat",
			},
			expectedServiceTag: "process_context:cat",
		},
		{
			name: "java jar with dd.Service",
			cmdline: []string{
				"/usr/lib/jvm/java-1.17.0-openjdk-amd64/bin/java", "-Dsun.misc.URLClassPath.disableJarChecking=true",
				"-Xms1024m", "-Xmx1024m", "-Dlogging.config=file:/usr/local/test/etc/logback-spring-datadog.xml",
				"-Dlog4j2.formatMsgNoLookups=true", "-javaagent:/opt/datadog-agent/dd-java-agent.jar",
				"-Ddd.profiling.enabled=true", "-Ddd.logs.injection=true", "-Ddd.trace.propagation.style.inject=datadog,b3multi",
				"-Ddd.rabbitmq.legacy.tracing.enabled=false", "-Ddd.service=myservice", "-jar",
				"/usr/local/test/app/myservice-core-1.1.15-SNAPSHOT.jar", "--spring.profiles.active=test",
			},
			expectedServiceTag: "process_context:myservice",
		},
		{
			name: "java with unknown flags",
			cmdline: []string{
				"java", "-Des.networkaddress.cache.ttl=60", "-Des.networkaddress.cache.negative.ttl=10",
				"-Djava.security.manager=allow", "-XX:+AlwaysPreTouch", "-Xss1m",
			},
			expectedServiceTag: "process_context:java",
		},
		{
			name: "java jar with snapshot",
			cmdline: []string{
				"/usr/lib/jvm/java-1.17.0-openjdk-amd64/bin/java", "-Dsun.misc.URLClassPath.disableJarChecking=true",
				"-Xms1024m", "-Xmx1024m", "-Dlogging.config=file:/usr/local/test/etc/logback-spring-datadog.xml",
				"-Dlog4j2.formatMsgNoLookups=true", "-javaagent:/opt/datadog-agent/dd-java-agent.jar",
				"-Ddd.profiling.enabled=true", "-Ddd.logs.injection=true", "-Ddd.trace.propagation.style.inject=datadog,b3multi",
				"-Ddd.rabbitmq.legacy.tracing.enabled=false", "-jar",
				"/usr/local/test/app/myservice-core-1.1.15-SNAPSHOT.jar", "--spring.profiles.active=test",
			},
			expectedServiceTag: "process_context:myservice-core",
		},
		{
			name: "java jar with snapshot with another version",
			cmdline: []string{
				"/usr/lib/jvm/java-1.17.0-openjdk-amd64/bin/java", "-Dsun.misc.URLClassPath.disableJarChecking=true",
				"-Xms1024m", "-Xmx1024m", "-Dlogging.config=file:/usr/local/test/etc/logback-spring-datadog.xml",
				"-Dlog4j2.formatMsgNoLookups=true", "-javaagent:/opt/datadog-agent/dd-java-agent.jar",
				"-Ddd.profiling.enabled=true", "-Ddd.logs.injection=true", "-Ddd.trace.propagation.style.inject=datadog,b3multi",
				"-Ddd.rabbitmq.legacy.tracing.enabled=false", "-jar",
				"/usr/local/test/app/myservice-core-1-SNAPSHOT.jar", "--spring.profiles.active=test",
			},
			expectedServiceTag: "process_context:myservice-core",
		},
		{
			name: "java jar with snapshot without version",
			cmdline: []string{
				"/usr/lib/jvm/java-1.17.0-openjdk-amd64/bin/java", "-Dsun.misc.URLClassPath.disableJarChecking=true",
				"-Xms1024m", "-Xmx1024m", "-Dlogging.config=file:/usr/local/test/etc/logback-spring-datadog.xml",
				"-Dlog4j2.formatMsgNoLookups=true", "-javaagent:/opt/datadog-agent/dd-java-agent.jar",
				"-Ddd.profiling.enabled=true", "-Ddd.logs.injection=true", "-Ddd.trace.propagation.style.inject=datadog,b3multi",
				"-Ddd.rabbitmq.legacy.tracing.enabled=false", "-jar",
				"/usr/local/test/app/myservice-core-SNAPSHOT.jar", "--spring.profiles.active=test",
			},
			expectedServiceTag: "process_context:myservice-core",
		},
		{
			name: "node js with advanced guess disabled",
			cmdline: []string{
				"/usr/bin/node",
				"--require",
				"/private/node-patches_legacy/register.js",
				"--preserve-symlinks-main",
				"--",
				"/somewhere/index.js",
			},
			expectedServiceTag: "process_context:node",
		},
		{
			name:                 "node js with advanced guess enabled with a broken package.json",
			useImprovedAlgorithm: true,
			cmdline: []string{
				"/usr/bin/node",
				"./nodejs/testData/inner/index.js",
			},
			expectedServiceTag: "process_context:node",
		},
		{
			name:                 "node js with advanced guess enabled and found a valid package.json",
			useImprovedAlgorithm: true,
			cmdline: []string{
				"/usr/bin/node",
				"--require",
				"/private/node-patches_legacy/register.js",
				"--preserve-symlinks-main",
				"--",
				"./nodejs/testData/index.js",
			},
			expectedServiceTag: "process_context:my-awesome-package",
		},
		{
			name: "dotnet cmd with dll",
			cmdline: []string{
				"/usr/bin/dotnet", "./myservice.dll",
			},
			useImprovedAlgorithm: true,
			expectedServiceTag:   "process_context:myservice",
		},
		{
			name: "dotnet cmd with dll and options",
			cmdline: []string{
				"/usr/bin/dotnet", "-v", "--", "/app/lib/myservice.dll",
			},
			useImprovedAlgorithm: true,
			expectedServiceTag:   "process_context:myservice",
		},
		{
			name: "dotnet cmd with unrecognized options",
			cmdline: []string{
				"/usr/bin/dotnet", "run", "--project", "./projects/proj1/proj1.csproj",
			},
			useImprovedAlgorithm: true,
			expectedServiceTag:   "process_context:dotnet",
		},
		{
			name: "dotnet cmd with improved algorithm disabled",
			cmdline: []string{
				"/usr/bin/dotnet", "./myservice.dll",
			},
			expectedServiceTag: "process_context:dotnet",
		},
		{
			name: "envs but no command",
			cmdline: []string{
				"ENV=VALUE",
			},
			expectedServiceTag: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proc := procutil.Process{
				Pid:     1,
				Cmdline: tt.cmdline,
			}
			procsByPid := map[int32]*procutil.Process{proc.Pid: &proc}
			serviceExtractorEnabled := true
			useWindowsServiceName := true
			useImprovedAlgorithm := tt.useImprovedAlgorithm
			se := NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)
			se.Extract(procsByPid)
			assert.Equal(t, []string{tt.expectedServiceTag}, se.GetServiceContext(proc.Pid))
		})
	}
}

func TestExtractServiceMetadataDisabled(t *testing.T) {
	proc := procutil.Process{
		Pid:     1,
		Cmdline: []string{"/bin/bash"},
	}
	procsByPid := map[int32]*procutil.Process{proc.Pid: &proc}
	serviceExtractorEnabled := false
	useWindowsServiceName := false
	useImprovedAlgorithm := false
	se := NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)
	se.Extract(procsByPid)
	assert.Empty(t, se.GetServiceContext(proc.Pid))
}

func TestChooseServiceNameFromEnvs(t *testing.T) {
	tests := []struct {
		name     string
		envs     []string
		expected string
		found    bool
	}{
		{
			name: "extract from DD_SERVICE",
			envs: []string{
				"DD_TRACE_DEBUG=true",
				"DD_TAGS=env:test",
				"DD_SERVICE=myservice",
			},
			expected: "myservice",
			found:    true,
		},
		{
			name: "extract from DD_TAGS",
			envs: []string{
				"DD_TAGS=env:test,dc:dc1,service:myservice",
			},
			expected: "myservice",
			found:    true,
		},
		{
			name: "could not extract from env",
			envs: []string{
				"DD_TRACE_DEBUG=true",
				"DD_TAGS=peer_service:test",
			},
			expected: "",
			found:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := chooseServiceNameFromEnvs(tt.envs)
			require.Equal(t, tt.expected, value)
			require.Equal(t, tt.found, ok)
		})
	}
}
