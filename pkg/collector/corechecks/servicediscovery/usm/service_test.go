// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"archive/zip"
	"errors"
	"io/fs"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"go.uber.org/zap"
)

const (
	springBootApp = "app/app.jar"

	// we need to use these non-descriptive shorter folder names because of the filename_linting
	// CI check that limits the number of characters in a path to 255.
	jbossTestAppRoot    = "../testdata/a"
	weblogicTestAppRoot = "../testdata/b"
)

func TestExtractServiceMetadata(t *testing.T) {
	springBootAppFullPath := createMockSpringBootApp(t)
	tests := []struct {
		name                       string
		cmdline                    []string
		envs                       []string
		expectedServiceTag         string
		expectedAdditionalServices []string
		fromDDService              bool
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
			expectedServiceTag: "my-server",
		},
		{
			name: "single arg executable with DD_SERVICE",
			cmdline: []string{
				"./my-server.sh",
			},
			envs:               []string{"DD_SERVICE=my-service"},
			expectedServiceTag: "my-service",
			fromDDService:      true,
		},
		{
			name: "single arg executable with DD_TAGS",
			cmdline: []string{
				"./my-server.sh",
			},
			envs:               []string{"DD_TAGS=service:my-service"},
			expectedServiceTag: "my-service",
			fromDDService:      true,
		},
		{
			name: "single arg executable with special chars",
			cmdline: []string{
				"./-my-server.sh-",
			},
			expectedServiceTag: "my-server",
		},
		{
			name: "sudo",
			cmdline: []string{
				"sudo", "-E", "-u", "dog", "/usr/local/bin/myApp", "-items=0,1,2,3", "-foo=bar",
			},
			expectedServiceTag: "myApp",
		},
		{
			name: "python flask argument",
			cmdline: []string{
				"/opt/python/2.7.11/bin/python2.7", "flask", "run", "--host=0.0.0.0",
			},
			expectedServiceTag: "flask",
			envs:               []string{"PWD=testdata/python"},
		},
		{
			name: "python - flask argument in path",
			cmdline: []string{
				"/opt/python/2.7.11/bin/python2.7", "testdata/python/flask", "run", "--host=0.0.0.0", "--without-threads",
			},
			expectedServiceTag: "flask",
		},
		{
			name: "python flask in single argument",
			cmdline: []string{
				"/opt/python/2.7.11/bin/python2.7 flask run --host=0.0.0.0",
			},
			envs:               []string{"PWD=testdata/python"},
			expectedServiceTag: "flask",
		},
		{
			name: "python - module hello",
			cmdline: []string{
				"python3", "-m", "hello",
			},
			expectedServiceTag: "hello",
		},
		{
			name: "ruby - td-agent",
			cmdline: []string{
				"ruby", "/usr/sbin/td-agent", "--log", "/var/log/td-agent/td-agent.log", "--daemon", "/var/run/td-agent/td-agent.pid",
			},
			expectedServiceTag: "td-agent",
		},
		{
			name: "Ruby on Rails with a valid application.rb file",
			cmdline: []string{
				"ruby",
				"bin/rails",
				"server",
			},
			envs:               []string{"PWD=testdata/rails"},
			expectedServiceTag: "my_http_rails_app_x",
		},
		{
			name: "java using the -jar flag to define the service",
			cmdline: []string{
				"java", "-Xmx4000m", "-Xms4000m", "-XX:ReservedCodeCacheSize=256m", "-jar", "/opt/sheepdog/bin/myservice.jar",
			},
			expectedServiceTag: "myservice",
		},
		{
			name: "java class name as service",
			cmdline: []string{
				"java", "-Xmx4000m", "-Xms4000m", "-XX:ReservedCodeCacheSize=256m", "com.datadog.example.HelloWorld",
			},
			expectedServiceTag: "HelloWorld",
		},
		{
			name: "java kafka",
			cmdline: []string{
				"java", "-Xmx4000m", "-Xms4000m", "-XX:ReservedCodeCacheSize=256m", "kafka.Kafka",
			},
			expectedServiceTag: "Kafka",
		},
		{
			name: "java parsing for org.apache projects with cassandra as the service",
			cmdline: []string{
				"/usr/bin/java", "-Xloggc:/usr/share/cassandra/logs/gc.log", "-ea", "-XX:+HeapDumpOnOutOfMemoryError", "-Xss256k", "-Dlogback.configurationFile=logback.xml",
				"-Dcassandra.logdir=/var/log/cassandra", "-Dcassandra.storagedir=/data/cassandra",
				"-cp", "/etc/cassandra:/usr/share/cassandra/lib/HdrHistogram-2.1.9.jar:/usr/share/cassandra/lib/cassandra-driver-core-3.0.1-shaded.jar",
				"org.apache.cassandra.service.CassandraDaemon",
			},
			expectedServiceTag: "cassandra",
		},
		{
			name: "java space in java executable path",
			cmdline: []string{
				"/home/dd/my java dir/java", "com.dog.cat",
			},
			expectedServiceTag: "cat",
		}, {
			name: "node js with package.json not present",
			cmdline: []string{
				"/usr/bin/node",
				"--require",
				"/private/node-patches_legacy/register.js",
				"--preserve-symlinks-main",
				"--",
				"/somewhere/index.js",
			},
			expectedServiceTag: "",
		},
		{
			name: "node js with a broken package.json",
			cmdline: []string{
				"/usr/bin/node",
				"./testdata/inner/index.js",
			},
			expectedServiceTag: "",
		},
		{
			name: "node js with a valid package.json",
			cmdline: []string{
				"/usr/bin/node",
				"--require",
				"/private/node-patches_legacy/register.js",
				"--preserve-symlinks-main",
				"--",
				"./testdata/index.js",
			},
			expectedServiceTag: "my-awesome-package",
		},
		{
			name: "node js with a valid nested package.json and cwd",
			cmdline: []string{
				"/usr/bin/node",
				"--require",
				"/private/node-patches_legacy/register.js",
				"--preserve-symlinks-main",
				"--",
				"index.js",
			},
			envs:               []string{"PWD=testdata/deep"}, // it's relative but it's ok for testing purposes
			expectedServiceTag: "my-awesome-package",
		},
		{
			name: "spring boot default options",
			cmdline: []string{
				"java",
				"-jar",
				springBootAppFullPath,
			},
			expectedServiceTag: "default-app",
		},
		{
			name: "wildfly 18 standalone",
			cmdline: []string{"home/app/.sdkman/candidates/java/17.0.4.1-tem/bin/java",
				"-D[Standalone]",
				"-server",
				"-Xms64m",
				"-Xmx512m",
				"-XX:MetaspaceSize=96M",
				"-XX:MaxMetaspaceSize=256m",
				"-Djava.net.preferIPv4Stack=true",
				"-Djboss.modules.system.pkgs=org.jboss.byteman",
				"-Djava.awt.headless=true",
				"--add-exports=java.base/sun.nio.ch=ALL-UNNAMED",
				"--add-exports=jdk.unsupported/sun.misc=ALL-UNNAMED",
				"--add-exports=jdk.unsupported/sun.reflect=ALL-UNNAMED",
				"-Dorg.jboss.boot.log.file=" + jbossTestAppRoot + "/standalone/log/server.log",
				"-Dlogging.configuration=file:" + jbossTestAppRoot + "/standalone/configuration/logging.properties",
				"-jar",
				"" + jbossTestAppRoot + "/jboss-modules.jar",
				"-mp",
				"" + jbossTestAppRoot + "/modules",
				"org.jboss.as.standalone",
				"-Djboss.home.dir=" + jbossTestAppRoot,
				"-Djboss.server.base.dir=" + jbossTestAppRoot + "/standalone"},
			expectedServiceTag:         "jboss-modules",
			expectedAdditionalServices: []string{"my-jboss-webapp", "some_context_root", "web3"},
		},
		{
			name: "wildfly 18 domain",
			cmdline: []string{"/home/app/.sdkman/candidates/java/17.0.4.1-tem/bin/java",
				"--add-exports=java.base/sun.nio.ch=ALL-UNNAMED",
				"--add-exports=jdk.unsupported/sun.reflect=ALL-UNNAMED",
				"--add-exports=jdk.unsupported/sun.misc=ALL-UNNAMED",
				"-D[Server:server-one]",
				"-D[pcid:780891833]",
				"-Xms64m",
				"-Xmx512m",
				"-server",
				"-XX:MetaspaceSize=96m",
				"-XX:MaxMetaspaceSize=256m",
				"-Djava.awt.headless=true",
				"-Djava.net.preferIPv4Stack=true",
				"-Djboss.home.dir=" + jbossTestAppRoot,
				"-Djboss.modules.system.pkgs=org.jboss.byteman",
				"-Djboss.server.log.dir=" + jbossTestAppRoot + "/domain/servers/server-one/log",
				"-Djboss.server.temp.dir=" + jbossTestAppRoot + "/domain/servers/server-one/tmp",
				"-Djboss.server.data.dir=" + jbossTestAppRoot + "/domain/servers/server-one/data",
				"-Dorg.jboss.boot.log.file=" + jbossTestAppRoot + "/domain/servers/server-one/log/server.log",
				"-Dlogging.configuration=file:" + jbossTestAppRoot + "/domain/configuration/default-server-logging.properties",
				"-jar",
				"" + jbossTestAppRoot + "/jboss-modules.jar",
				"-mp",
				"" + jbossTestAppRoot + "/modules",
				"org.jboss.as.server"},
			expectedServiceTag:         "jboss-modules",
			expectedAdditionalServices: []string{"web3", "web4"},
		},
		{
			name: "weblogic 12",
			cmdline: []string{"/u01/jdk/bin/java",
				"-Djava.security.egd=file:/dev/./urandom",
				"-cp",
				"/u01/oracle/wlserver/server/lib/weblogic-launcher.jar",
				"-Dlaunch.use.env.classpath=true",
				"-Dweblogic.Name=AdminServer",
				"-Djava.security.policy=/u01/oracle/wlserver/server/lib/weblogic.policy",
				"-Djava.system.class.loader=com.oracle.classloader.weblogic.LaunchClassLoader",
				"-javaagent:/u01/oracle/wlserver/server/lib/debugpatch-agent.jar",
				"-da",
				"-Dwls.home=/u01/oracle/wlserver/server",
				"-Dweblogic.home=/u01/oracle/wlserver/server",
				"weblogic.Server"},
			envs:                       []string{"PWD=" + weblogicTestAppRoot},
			expectedServiceTag:         "Server",
			expectedAdditionalServices: []string{"my_context", "sample4", "some_context_root"},
		},
		{
			name: "java with dd_service as system property",
			cmdline: []string{
				"/usr/bin/java", "-Ddd.service=custom", "-jar", "app.jar",
			},
			expectedServiceTag: "custom",
			fromDDService:      true,
		},
		{
			name: "Tomcat 10.X",
			cmdline: []string{
				"/usr/bin/java",
				"-Djava.util.logging.config.file=testdata/tomcat/conf/logging.properties",
				"-Djava.util.logging.manager=org.apache.juli.ClassLoaderLogManager",
				"-Djdk.tls.ephemeralDHKeySize=2048",
				"-Djava.protocol.handler.pkgs=org.apache.catalina.webresources",
				"-Dorg.apache.catalina.security.SecurityListener.UMASK=0027",
				"--add-opens=java.base/java.lang=ALL-UNNAMED",
				"--add-opens=java.base/java.io=ALL-UNNAMED",
				"--add-opens=java.base/java.util=ALL-UNNAMED",
				"--add-opens=java.base/java.util.concurrent=ALL-UNNAMED",
				"--add-opens=java.rmi/sun.rmi.transport=ALL-UNNAMED",
				"-classpath",
				"testdata/tomcat/bin/bootstrap.jar:testdata/tomcat/bin/tomcat-juli.jar",
				"-Dcatalina.base=testdata/tomcat",
				"-Dcatalina.home=testdata/tomcat",
				"-Djava.io.tmpdir=testdata/tomcat/temp",
				"org.apache.catalina.startup.Bootstrap",
				"start",
			},
			expectedServiceTag:         "catalina",
			expectedAdditionalServices: []string{"app2", "custom"},
		},
		{
			name: "dotnet cmd with dll",
			cmdline: []string{
				"/usr/bin/dotnet", "./myservice.dll",
			},
			expectedServiceTag: "myservice",
		},
		{
			name: "dotnet cmd with dll and options",
			cmdline: []string{
				"/usr/bin/dotnet", "-v", "--", "/app/lib/myservice.dll",
			},
			expectedServiceTag: "myservice",
		},
		{
			name: "dotnet cmd with unrecognized options",
			cmdline: []string{
				"/usr/bin/dotnet", "run", "--project", "./projects/proj1/proj1.csproj",
			},
		},
		{
			name: "PHP Laravel",
			cmdline: []string{
				"php",
				"artisan",
				"serve",
			},
			expectedServiceTag: "laravel",
		},
		{
			name: "Plain PHP with INI",
			cmdline: []string{
				"php",
				"-ddatadog.service=foo",
				"swoole-server.php",
			},
			expectedServiceTag: "foo",
		},
		{
			name: "PHP with version number",
			cmdline: []string{
				"php8.3",
				"artisan",
				"migrate:fresh",
			},
			expectedServiceTag: "laravel",
		},
		{
			name: "PHP with two-digit version number",
			cmdline: []string{
				"php8.10",
				"artisan",
				"migrate:fresh",
			},
			expectedServiceTag: "laravel",
		},
		{
			name: "PHP-FPM shouldn't trigger php parsing",
			cmdline: []string{
				"php-fpm",
				"artisan",
			},
			expectedServiceTag: "php-fpm",
		},
		{
			name: "PHP-FPM with version number shouldn't trigger php parsing",
			cmdline: []string{
				"php8.1-fpm",
				"artisan",
			},
			expectedServiceTag: "php8",
		},
		{
			name:               "DD_SERVICE_set_manually",
			cmdline:            []string{"java", "-jar", "Foo.jar"},
			envs:               []string{"DD_SERVICE=howdy"},
			expectedServiceTag: "howdy",
			fromDDService:      true,
		},
		{
			name:               "DD_SERVICE_set_manually_tags",
			cmdline:            []string{"java", "-jar", "Foo.jar"},
			envs:               []string{"DD_TAGS=service:howdy"},
			expectedServiceTag: "howdy",
			fromDDService:      true,
		},
		{
			name:               "DD_SERVICE_set_manually_injection",
			cmdline:            []string{"java", "-jar", "Foo.jar"},
			envs:               []string{"DD_SERVICE=howdy", "DD_INJECTION_ENABLED=tracer,service_name"},
			expectedServiceTag: "howdy",
			fromDDService:      false,
		},
		{
			name: "gunicorn simple",
			cmdline: []string{
				"gunicorn",
				"--workers=2",
				"test:app",
			},
			expectedServiceTag: "test",
		},
		{
			name: "gunicorn from name",
			cmdline: []string{
				"gunicorn",
				"--workers=2",
				"-b",
				"0.0.0.0",
				"-n",
				"dummy",
				"test:app",
			},
			expectedServiceTag: "dummy",
		},
		{
			name: "gunicorn from name (long arg)",
			cmdline: []string{
				"gunicorn",
				"--workers=2",
				"-b",
				"0.0.0.0",
				"--name=dummy",
				"test:app",
			},
			expectedServiceTag: "dummy",
		},
		{
			name: "gunicorn from name in env",
			cmdline: []string{
				"gunicorn",
				"test:app",
			},
			envs:               []string{"GUNICORN_CMD_ARGS=--bind=127.0.0.1:8080 --workers=3 -n dummy"},
			expectedServiceTag: "dummy",
		},
		{
			name: "gunicorn without app found",
			cmdline: []string{
				"gunicorn",
			},
			envs:               []string{"GUNICORN_CMD_ARGS=--bind=127.0.0.1:8080 --workers=3"},
			expectedServiceTag: "gunicorn",
		},
		{
			name: "gunicorn with partial wsgi app",
			cmdline: []string{
				"gunicorn",
				"my.package",
			},
			expectedServiceTag: "my.package",
		},
		{
			name: "gunicorn with empty WSGI_APP env",
			cmdline: []string{
				"gunicorn",
				"my.package",
			},
			envs:               []string{"WSGI_APP="},
			expectedServiceTag: "my.package",
		},
		{
			name: "gunicorn with WSGI_APP env",
			cmdline: []string{
				"gunicorn",
			},
			envs:               []string{"WSGI_APP=test:app"},
			expectedServiceTag: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, ok := ExtractServiceMetadata(zap.NewNop(), tt.cmdline, tt.envs)
			if len(tt.expectedServiceTag) == 0 {
				require.False(t, ok)
			} else {
				require.True(t, ok)
				require.Equal(t, tt.expectedServiceTag, meta.Name)
				require.Equal(t, tt.expectedAdditionalServices, meta.AdditionalNames)
				require.Equal(t, tt.fromDDService, meta.FromDDService)
			}
		})
	}
}

func writeFile(writer *zip.Writer, name string, content string) error {
	w, err := writer.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(content))
	return err
}

type chainedFS struct {
	chain []fs.FS
}

func (c chainedFS) Open(name string) (fs.File, error) {
	var err error
	for _, current := range c.chain {
		var f fs.File
		f, err = current.Open(name)
		if err == nil {
			return f, nil
		}
	}
	return nil, err
}

func (c chainedFS) Sub(dir string) (fs.FS, error) {
	for _, current := range c.chain {
		if sub, ok := current.(fs.SubFS); ok {
			return sub.Sub(dir)
		}
	}
	return nil, errors.New("no suitable SubFS in the chain")
}

type shadowFS struct {
	filesystem fs.FS
	parent     fs.FS
	globs      []string
}

func (s shadowFS) Open(name string) (fs.File, error) {
	var fsys fs.FS
	if s.parent != nil {
		fsys = s.parent
	} else {
		fsys = s.filesystem
	}
	for _, current := range s.globs {
		ok, err := path.Match(current, name)
		if err != nil {
			return nil, err
		}
		if ok {
			return nil, fs.ErrNotExist
		}
	}
	return fsys.Open(name)
}

func (s shadowFS) Sub(dir string) (fs.FS, error) {
	fsys, err := fs.Sub(s.filesystem, dir)
	if err != nil {
		return nil, err
	}
	return shadowFS{filesystem: fsys, parent: s}, nil
}
