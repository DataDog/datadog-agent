// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assertClean(t *testing.T, contents, cleanContents string) {
	cleaned, err := ScrubBytes([]byte(contents))
	assert.NoError(t, err)
	cleanedString := string(cleaned)

	assert.Equal(t, strings.TrimSpace(cleanContents), strings.TrimSpace(cleanedString))
}

func TestConfigStripApiKey(t *testing.T) {
	assertClean(t,
		`api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`api_key: "***************************abbbb"`)
	assertClean(t,
		`api_key: AAAAAAAAAAAAAAAAAAAAAAAAAAAABBBB`,
		`api_key: "***************************ABBBB"`)
	assertClean(t,
		`api_key: "aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb"`,
		`api_key: "***************************abbbb"`)
	assertClean(t,
		`api_key: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb'`,
		`api_key: '***************************abbbb'`)
	assertClean(t,
		`api_key: |
			aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`api_key: |
			***************************abbbb`)
	assertClean(t,
		`api_key: >
			aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`api_key: >
			***************************abbbb`)
	assertClean(t,
		`   api_key:   'aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb'   `,
		`   api_key:   '***************************abbbb'   `)
	assertClean(t,
		`
		additional_endpoints:
			"https://app.datadoghq.com":
			- aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb,
			- bbbbbbbbbbbbbbbbbbbbbbbbbbbbaaaa,
			"https://dog.datadoghq.com":
			- aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb,
			- bbbbbbbbbbbbbbbbbbbbbbbbbbbbaaaa`,
		`
		additional_endpoints:
			"https://app.datadoghq.com":
			- "***************************abbbb",
			- "***************************baaaa",
			"https://dog.datadoghq.com":
			- "***************************abbbb",
			- "***************************baaaa"`)
	// make sure we don't strip container ids
	assertClean(t,
		`container_id: "b32bd6f9b73ba7ccb64953a04b82b48e29dfafab65fd57ca01d3b94a0e024885"`,
		`container_id: "b32bd6f9b73ba7ccb64953a04b82b48e29dfafab65fd57ca01d3b94a0e024885"`)
}

func TestConfigAppKey(t *testing.T) {
	assertClean(t,
		`app_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`app_key: "***********************************abbbb"`)
	assertClean(t,
		`app_key: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABBBB`,
		`app_key: "***********************************ABBBB"`)
	assertClean(t,
		`app_key: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb"`,
		`app_key: "***********************************abbbb"`)
	assertClean(t,
		`app_key: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb'`,
		`app_key: '***********************************abbbb'`)
	assertClean(t,
		`app_key: |
			aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`app_key: |
			***********************************abbbb`)
	assertClean(t,
		`app_key: >
			aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`app_key: >
			***********************************abbbb`)
	assertClean(t,
		`   app_key:   'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb'   `,
		`   app_key:   '***********************************abbbb'   `)
}

func TestConfigRCAppKey(t *testing.T) {
	assertClean(t,
		`key: "DDRCM_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABCDE"`,
		`key: "***********************************ABCDE"`)
}

func TestConfigStripURLPassword(t *testing.T) {
	assertClean(t,
		`proxy: random_url_key: http://user:password@host:port`,
		`proxy: random_url_key: http://user:********@host:port`)
	assertClean(t,
		`random_url_key http://user:password@host:port`,
		`random_url_key http://user:********@host:port`)
	assertClean(t,
		`random_url_key: http://user:password@host:port`,
		`random_url_key: http://user:********@host:port`)
	assertClean(t,
		`random_url_key: http://user:p@ssw0r)@host:port`,
		`random_url_key: http://user:********@host:port`)
	assertClean(t,
		`random_url_key: http://user:🔑🔒🔐🔓@host:port`,
		`random_url_key: http://user:********@host:port`)
	assertClean(t,
		`random_url_key: http://user:password@host`,
		`random_url_key: http://user:********@host`)
	assertClean(t,
		`random_url_key: protocol://user:p@ssw0r)@host:port`,
		`random_url_key: protocol://user:********@host:port`)
	assertClean(t,
		`random_url_key: "http://user:password@host:port"`,
		`random_url_key: "http://user:********@host:port"`)
	assertClean(t,
		`random_url_key: 'http://user:password@host:port'`,
		`random_url_key: 'http://user:********@host:port'`)
	assertClean(t,
		`random_domain_key: 'user:password@host:port'`,
		`random_domain_key: 'user:********@host:port'`)
	assertClean(t,
		`random_url_key: |
			http://user:password@host:port`,
		`random_url_key: |
			http://user:********@host:port`)
	assertClean(t,
		`random_url_key: >
			http://user:password@host:port`,
		`random_url_key: >
			http://user:********@host:port`)
	assertClean(t,
		`   random_url_key:   'http://user:password@host:port'   `,
		`   random_url_key:   'http://user:********@host:port'   `)
	assertClean(t,
		`   random_url_key:   'mongodb+s.r-v://user:password@host:port'   `,
		`   random_url_key:   'mongodb+s.r-v://user:********@host:port'   `)
	assertClean(t,
		`   random_url_key:   'mongodb+srv://user:pass-with-hyphen@abc.example.com/database'   `,
		`   random_url_key:   'mongodb+srv://user:********@abc.example.com/database'   `)
	assertClean(t,
		`   random_url_key:   'http://user-with-hyphen:pass-with-hyphen@abc.example.com/database'   `,
		`   random_url_key:   'http://user-with-hyphen:********@abc.example.com/database'   `)
	assertClean(t,
		`   random_url_key:   'http://user-with-hyphen:pass@abc.example.com/database'   `,
		`   random_url_key:   'http://user-with-hyphen:********@abc.example.com/database'   `)

	assertClean(t,
		`flushing serie: {"metric":"kubeproxy","tags":["image_id":"foobar/foobaz@sha256:e8dabc7d398d25ecc8a3e33e3153e988e79952f8783b81663feb299ca2d0abdd"]}`,
		`flushing serie: {"metric":"kubeproxy","tags":["image_id":"foobar/foobaz@sha256:e8dabc7d398d25ecc8a3e33e3153e988e79952f8783b81663feb299ca2d0abdd"]}`)

	assertClean(t,
		`"simple.metric:44|g|@1.00000"`,
		`"simple.metric:44|g|@1.00000"`)
}

func TestTextStripApiKey(t *testing.T) {
	assertClean(t,
		`Error status code 500 : http://dog.tld/api?key=3290abeefc68e1bbe852a25252bad88c`,
		`Error status code 500 : http://dog.tld/api?key=***************************ad88c`)
	assertClean(t,
		`hintedAPIKeyReplacer : http://dog.tld/api_key=InvalidLength12345abbbb`,
		`hintedAPIKeyReplacer : http://dog.tld/api_key=***************************abbbb`)
	assertClean(t,
		`hintedAPIKeyReplacer : http://dog.tld/apikey=InvalidLength12345abbbb`,
		`hintedAPIKeyReplacer : http://dog.tld/apikey=***************************abbbb`)
	assertClean(t,
		`apiKeyReplacer: https://agent-http-intake.logs.datadoghq.com/v1/input/aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`apiKeyReplacer: https://agent-http-intake.logs.datadoghq.com/v1/input/***************************abbbb`)
}

func TestTextStripAppKey(t *testing.T) {
	assertClean(t,
		`hintedAPPKeyReplacer : http://dog.tld/app_key=InvalidLength12345abbbb`,
		`hintedAPPKeyReplacer : http://dog.tld/app_key=***********************************abbbb`)
	assertClean(t,
		`hintedAPPKeyReplacer : http://dog.tld/appkey=InvalidLength12345abbbb`,
		`hintedAPPKeyReplacer : http://dog.tld/appkey=***********************************abbbb`)
	assertClean(t,
		`hintedAPPKeyReplacer : http://dog.tld/application_key=InvalidLength12345abbbb`,
		`hintedAPPKeyReplacer : http://dog.tld/application_key=***********************************abbbb`)
	assertClean(t,
		`appKeyReplacer: http://dog.tld/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`appKeyReplacer: http://dog.tld/***********************************abbbb`)
}

func TestTextStripURLPassword(t *testing.T) {
	assertClean(t,
		`Connection dropped : ftp://user:password@host:port`,
		`Connection dropped : ftp://user:********@host:port`)
}

func TestTextStripLogPassword(t *testing.T) {
	testcases := []struct {
		name string
		log  string
		want string
	}{
		{
			name: "logged uppercase (eg. Password=)",
			log:  `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: Username=userme Password=$AeVtn8*gbyaf!hnUHx^L."`,
			want: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: Username=userme Password=********"`,
		},
		{
			name: "logged lowercase (eg. password=)",
			log:  `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: username=userme password=$AeVtn8*gbyaf!hnUHx^L."`,
			want: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: username=userme password=********"`,
		},
		{
			name: "logged with whitespace (eg. password =)",
			log:  `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: username = userme password = $AeVtn8*gbyaf!hnUHx^L."`,
			want: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: username = userme password = ********"`,
		},
		{
			name: "logged as json (eg. \"Password\":)",
			log:  `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: {"username": "userme",  "password": "$AeVtn8*gbyaf!hnUHx^L."}"`,
			want: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: {"username": "userme",  "password": "********"}"`,
		},
		{
			name: "logged as json with single quotes (eg. 'password':)",
			log:  `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: {'username': 'userme',  'password': '$AeVtn8*gbyaf!hnUHx^L.'}`,
			want: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: {'username': 'userme',  'password': '********'}`,
		},
		{
			name: "single-quoted key with equals (eg. 'password'=)",
			log:  `2024-07-02 10:40:18 EDT | CORE | ERROR | Error: 'password'=MyS3cr3t!Pass user='admin'`,
			want: `2024-07-02 10:40:18 EDT | CORE | ERROR | Error: 'password'=******** user='admin'`,
		},
		{
			name: "single-quoted pwd key (eg. 'pwd':)",
			log:  `Connection config: {'host': 'localhost', 'pwd': 'Secr3t@123', 'port': 5432}`,
			want: `Connection config: {'host': 'localhost', 'pwd': '********', 'port': 5432}`,
		},
		{
			name: "single-quoted pswd key (eg. 'pswd'=)",
			log:  `Auth failed: 'pswd'=P@ssw0rd123`,
			want: `Auth failed: 'pswd'=********`,
		},
		{
			name: "logged PSWD (eg. PSWD=)",
			log:  `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: USER=userme PSWD=$AeVtn8*gbyaf!hnUHx^L."`,
			want: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: USER=userme PSWD=********"`,
		},
		{
			name: "already scrubbed log (eg. password=********)",
			log:  `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: username=userme password=********"`,
			want: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: username=userme password=********"`,
		},
		{
			name: "already scrubbed YAML (eg. password: ********)",
			log: `dd_url: https://app.datadoghq.com
api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
proxy: http://user:password@host:port
password: foo`,
			want: `dd_url: https://app.datadoghq.com
api_key: "***************************aaaaa"
proxy: http://user:********@host:port
password: "********"`,
		},
		{
			name: "real log test 1",
			log:  `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect to SQL Server, see https://docs.datadoghq.com/database_monitoring/setup_sql_server/troubleshooting#common-driver-issues for more details on how to debug this issue. TCP-connection(OK), Exception: OperationalError(com_error(-2147352567, 'Exception occurred.', (0, 'ADODB.Connection', 'Provider cannot be found. It may not be properly installed.', 'C:\\\\Windows\\\\HELP\\\\ADOIRAMG.CHM', 1240655, -2146824582), None), 'Error opening connection to \"ConnectRetryCount=2;Provider=MSOLEDBSQL;Data Source=fwurgdae532sk1,1433;Initial Catalog=master;User ID=sqlsqlsql;Password=Si5123$#!@as\\\\rrrrg;\"')\ncode=common-driver-issues connection_host=fwurgdae532sk1,1433 connector=adodbapi database=master driver=None host=fwurgdae532sk1,1433", "traceback": "Traceback (most recent call last):\n  File \"C:\\Program Files\\Datadog\\Datadog Agent\\embedded3\\Lib\\site-packages\\datadog_checks\\base\\checks\\base.py\", line 1210, in run\n    initialization()\n  File \"C:\\Program Files\\Datadog\\Datadog Agent\\embedded3\\Lib\\site-packages\\datadog_checks\\sqlserver\\sqlserver.py\", line 238, in set_resolved_hostname\n    self.load_static_information()\n  File \"C:\\Program Files\\Datadog\\Datadog Agent\\embedded3\\Lib\\site-packages\\datadog_checks\\sqlserver\\sqlserver.py\", line 277, in load_static_information\n    with self.connection.open_managed_default_connection():\n  File \"C:\\Program Files\\Datadog\\Datadog Agent\\embedded3\\Lib\\contextlib.py\", line 137, in __enter__\n    return next(self.gen)\n           ^^^^^^^^^^^^^^\n  File \"C:\\Program Files\\Datadog\\Datadog Agent\\embedded3\\Lib\\site-packages\\datadog_checks\\sqlserver\\connection.py\", line 227, in open_managed_default_connection\n    with self._open_managed_db_connections(self.DEFAULT_DB_KEY, key_prefix=key_prefix):\n  File \"C:\\Program Files\\Datadog\\Datadog Agent\\embedded3\\Lib\\contextlib.py\", line 137, in __enter__\n    return next(self.gen)\n           ^^^^^^^^^^^^^^\n  File \"C:\\Program Files\\Datadog\\Datadog Agent\\embedded3\\Lib\\site-packages\\datadog_checks\\sqlserver\\connection.py\", line 232, in _open_managed_db_connections\n    self.open_db_connections(db_key, db_name, key_prefix=key_prefix)\n  File \"C:\\Program Files\\Datadog\\Datadog Agent\\embedded3\\Lib\\site-packages\\datadog_checks\\sqlserver\\connection.py\", line 315, in open_db_connections\n    raise_from(SQLConnectionError(check_err_message), None)\n  File \"<string>\", line 3, in raise_from\ndatadog_checks.sqlserver.connection_errors.SQLConnectionError: Unable to connect to SQL Server, see https://docs.datadoghq.com/database_monitoring/setup_sql_server/troubleshooting#common-driver-issues for more details on how to debug this issue. TCP-connection(OK), Exception: OperationalError(com_error(-2147352567, 'Exception occurred.', (0, 'ADODB.Connection', 'Provider cannot be found. It may not be properly installed.', 'C:\\\\Windows\\\\HELP\\\\ADOIRAMG.CHM', 1240655, -2146824582), None), 'Error opening connection to \"ConnectRetryCount=2;Provider=MSOLEDBSQL;Data Source=fwurgdae532sk1,1433;Initial Catalog=master;User ID=sqlsqlsql;Password=Si5123$#!@as\\\\rrrrg;\"')\ncode=common-driver-issues connection_host=fwurgdae532sk1,1433 connector=adodbapi database=master driver=None host=lukprdsql51,1433\n"}]`,
			want: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect to SQL Server, see https://docs.datadoghq.com/database_monitoring/setup_sql_server/troubleshooting#common-driver-issues for more details on how to debug this issue. TCP-connection(OK), Exception: OperationalError(com_error(-2147352567, 'Exception occurred.', (0, 'ADODB.Connection', 'Provider cannot be found. It may not be properly installed.', 'C:\\\\Windows\\\\HELP\\\\ADOIRAMG.CHM', 1240655, -2146824582), None), 'Error opening connection to \"ConnectRetryCount=2;Provider=MSOLEDBSQL;Data Source=fwurgdae532sk1,1433;Initial Catalog=master;User ID=sqlsqlsql;Password=********;\"')\ncode=common-driver-issues connection_host=fwurgdae532sk1,1433 connector=adodbapi database=master driver=None host=fwurgdae532sk1,1433", "traceback": "Traceback (most recent call last):\n  File \"C:\\Program Files\\Datadog\\Datadog Agent\\embedded3\\Lib\\site-packages\\datadog_checks\\base\\checks\\base.py\", line 1210, in run\n    initialization()\n  File \"C:\\Program Files\\Datadog\\Datadog Agent\\embedded3\\Lib\\site-packages\\datadog_checks\\sqlserver\\sqlserver.py\", line 238, in set_resolved_hostname\n    self.load_static_information()\n  File \"C:\\Program Files\\Datadog\\Datadog Agent\\embedded3\\Lib\\site-packages\\datadog_checks\\sqlserver\\sqlserver.py\", line 277, in load_static_information\n    with self.connection.open_managed_default_connection():\n  File \"C:\\Program Files\\Datadog\\Datadog Agent\\embedded3\\Lib\\contextlib.py\", line 137, in __enter__\n    return next(self.gen)\n           ^^^^^^^^^^^^^^\n  File \"C:\\Program Files\\Datadog\\Datadog Agent\\embedded3\\Lib\\site-packages\\datadog_checks\\sqlserver\\connection.py\", line 227, in open_managed_default_connection\n    with self._open_managed_db_connections(self.DEFAULT_DB_KEY, key_prefix=key_prefix):\n  File \"C:\\Program Files\\Datadog\\Datadog Agent\\embedded3\\Lib\\contextlib.py\", line 137, in __enter__\n    return next(self.gen)\n           ^^^^^^^^^^^^^^\n  File \"C:\\Program Files\\Datadog\\Datadog Agent\\embedded3\\Lib\\site-packages\\datadog_checks\\sqlserver\\connection.py\", line 232, in _open_managed_db_connections\n    self.open_db_connections(db_key, db_name, key_prefix=key_prefix)\n  File \"C:\\Program Files\\Datadog\\Datadog Agent\\embedded3\\Lib\\site-packages\\datadog_checks\\sqlserver\\connection.py\", line 315, in open_db_connections\n    raise_from(SQLConnectionError(check_err_message), None)\n  File \"<string>\", line 3, in raise_from\ndatadog_checks.sqlserver.connection_errors.SQLConnectionError: Unable to connect to SQL Server, see https://docs.datadoghq.com/database_monitoring/setup_sql_server/troubleshooting#common-driver-issues for more details on how to debug this issue. TCP-connection(OK), Exception: OperationalError(com_error(-2147352567, 'Exception occurred.', (0, 'ADODB.Connection', 'Provider cannot be found. It may not be properly installed.', 'C:\\\\Windows\\\\HELP\\\\ADOIRAMG.CHM', 1240655, -2146824582), None), 'Error opening connection to \"ConnectRetryCount=2;Provider=MSOLEDBSQL;Data Source=fwurgdae532sk1,1433;Initial Catalog=master;User ID=sqlsqlsql;Password=********;\"')\ncode=common-driver-issues connection_host=fwurgdae532sk1,1433 connector=adodbapi database=master driver=None host=lukprdsql51,1433\n"}]`,
		},
		{
			name: "real log test 2",
			log:  `2024-03-22 02:00:09 EDT | PROCESS | WARN | (pkg/process/procutil/process_windows.go:603 in ParseCmdLineArgs) | unexpected quotes in string, giving up ( /SQL "\"\OwnerDashboard\"" /SERVER fwurgdae532sk1 /CONNECTION EDWPRD;"\"Data Source=EDWPRD;User ID=qwimMAK;password=quark0n;Persist Security Info=True;Unicode=True;\"" /CONNECTION MRDPRD;"\"Data Source=MRDPRD;User ID=quirmak;password=s3llerj4m;Persist Security Info=True;Unicode=True;\"" /CONNECTION RPTEJMACPRD;"\"Data Source=RPTEJMACPRD;User ID=vwurpe;password=squ1rr3l;Persist Security Info=True;Unicode=True;\"" /CHECKPOINTING OFF /REPORTING E)`,
			want: `2024-03-22 02:00:09 EDT | PROCESS | WARN | (pkg/process/procutil/process_windows.go:603 in ParseCmdLineArgs) | unexpected quotes in string, giving up ( /SQL "\"\OwnerDashboard\"" /SERVER fwurgdae532sk1 /CONNECTION EDWPRD;"\"Data Source=EDWPRD;User ID=qwimMAK;password=********;Persist Security Info=True;Unicode=True;\"" /CONNECTION MRDPRD;"\"Data Source=MRDPRD;User ID=quirmak;password=********;Persist Security Info=True;Unicode=True;\"" /CONNECTION RPTEJMACPRD;"\"Data Source=RPTEJMACPRD;User ID=vwurpe;password=********;Persist Security Info=True;Unicode=True;\"" /CHECKPOINTING OFF /REPORTING E)`,
		},
		{
			name: "real log test 3",
			log:  `2024-05-14 00:05:01 BST | PROCESS | WARN | (pkg/process/procutil/process_windows.go:603 in ParseCmdLineArgs) | unexpected quotes in string, giving up (""C:\User\shared\jdk\open-jdk-8-win32\1.8"\bin\javaw"  -Djava.library.path="C:\windows\system32;C:\windows;C:\windows\System32\Wbem;C:\windows\System32\WindowsPowerShell\v1.0\;C:\windows\System32\OpenSSH\;C:\Users\thufpos\AppData\Local\Microsoft\WindowsApps;;C:\User\\pos\licence;C:\User\\pos\shared-obj;C:\User\\pos\jpos-lib"	-Xms512M -Xmx1024M	-XX:+HeapDumpOnOutOfMemoryError -XX:HeapDumpPath="C:\User\\pos\logs"		-Djavax.net.ssl.trustStore="C:\User\\pos\trust\.mobilePOS.trustStore"	-Djavax.net.ssl.trustStorePassword="QwermiAD#@1sdkjf#$%\\xsdf|f!"	-Denactor.forceAutoGenClassesDeploy="true" -Denactor.autoGenClassesFolder="pos"		-Dhttps.protocols=TLSv1.1,TLSv1.2	-Djdk.tls.client.protocols="TLSv1.1,TLSv1.2"	-cp ";C:\User\\pos\config;C:\User\\pos\ext-lib\*;C:\User\\pos\custom-lib\*;C:\User\\pos\jdbc\*;C:\User\\pos\enactor-lib\*;C:\User\\pos\shared-obj\*;C:\User\\pos\encrypted-lib\*;C:\User\\pos\jpos\*;C:\User\\pos\jpos-lib\*;"	com.enactor.pos.swing.SwingPosApplication		-noDeploy)`,
			want: `2024-05-14 00:05:01 BST | PROCESS | WARN | (pkg/process/procutil/process_windows.go:603 in ParseCmdLineArgs) | unexpected quotes in string, giving up (""C:\User\shared\jdk\open-jdk-8-win32\1.8"\bin\javaw"  -Djava.library.path="C:\windows\system32;C:\windows;C:\windows\System32\Wbem;C:\windows\System32\WindowsPowerShell\v1.0\;C:\windows\System32\OpenSSH\;C:\Users\thufpos\AppData\Local\Microsoft\WindowsApps;;C:\User\\pos\licence;C:\User\\pos\shared-obj;C:\User\\pos\jpos-lib"	-Xms512M -Xmx1024M	-XX:+HeapDumpOnOutOfMemoryError -XX:HeapDumpPath="C:\User\\pos\logs"		-Djavax.net.ssl.trustStore="C:\User\\pos\trust\.mobilePOS.trustStore"	-Djavax.net.ssl.trustStorePassword="********"	-Denactor.forceAutoGenClassesDeploy="true" -Denactor.autoGenClassesFolder="pos"		-Dhttps.protocols=TLSv1.1,TLSv1.2	-Djdk.tls.client.protocols="TLSv1.1,TLSv1.2"	-cp ";C:\User\\pos\config;C:\User\\pos\ext-lib\*;C:\User\\pos\custom-lib\*;C:\User\\pos\jdbc\*;C:\User\\pos\enactor-lib\*;C:\User\\pos\shared-obj\*;C:\User\\pos\encrypted-lib\*;C:\User\\pos\jpos\*;C:\User\\pos\jpos-lib\*;"	com.enactor.pos.swing.SwingPosApplication		-noDeploy)`,
		},
		{
			name: "real log test 4",
			log:  `2024-07-05 14:36:54 CEST | CORE | DEBUG | (pkg/collector/python/datadog_agent.go:135 in LogMessage) | http_check: login (via catalog):ef29cea7c32fc55 | (http_check.py:119) | Connecting to http://catalog.example.com:8080/search-engine/login.do?userName=Morbotron&userPassword=K#2asdfu!23%%x`,
			want: `2024-07-05 14:36:54 CEST | CORE | DEBUG | (pkg/collector/python/datadog_agent.go:135 in LogMessage) | http_check: login (via catalog):ef29cea7c32fc55 | (http_check.py:119) | Connecting to http://catalog.example.com:8080/search-engine/login.do?userName=Morbotron&userPassword=********`,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			assertClean(t, tc.log, tc.want)
		})
	}
}

func TestDockerSelfInspectApiKey(t *testing.T) {
	assertClean(t,
		`
	"Env": [
		"DD_API_KEY=3290abeefc68e1bbe852a25252bad88c",
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"DOCKER_DD_AGENT=yes",
		"AGENT_VERSION=1:6.0",
		"DD_AGENT_HOME=/opt/datadog-agent6/"
	]`,
		`
	"Env": [
		"DD_API_KEY=***************************ad88c",
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"DOCKER_DD_AGENT=yes",
		"AGENT_VERSION=1:6.0",
		"DD_AGENT_HOME=/opt/datadog-agent6/"
	]`)
}

func TestConfigPassword(t *testing.T) {
	assertClean(t,
		`mysql_password: password`,
		`mysql_password: "********"`)
	assertClean(t,
		`mysql_pass: password`,
		`mysql_pass: "********"`)
	assertClean(t,
		`password_mysql: password`,
		`password_mysql: "********"`)
	assertClean(t,
		`mysql_password: p@ssw0r)`,
		`mysql_password: "********"`)
	assertClean(t,
		`mysql_password: 🔑 🔒 🔐 🔓`,
		`mysql_password: "********"`)
	assertClean(t,
		`mysql_password: password`,
		`mysql_password: "********"`)
	assertClean(t,
		`mysql_password: p@ssw0r)`,
		`mysql_password: "********"`)
	assertClean(t,
		`mysql_password: "password"`,
		`mysql_password: "********"`)
	assertClean(t,
		`mysql_password: 'password'`,
		`mysql_password: "********"`)
	assertClean(t,
		`   mysql_password:   'password'   `,
		`   mysql_password: "********"`)
	assertClean(t,
		`pwd: 'password'`,
		`pwd: "********"`)
	assertClean(t,
		`pwd: p@ssw0r`,
		`pwd: "********"`)
	assertClean(t,
		`cert_key_password: p@ssw0r`,
		`cert_key_password: "********"`)
	assertClean(t,
		`cert_key_password: 🔑 🔒 🔐 🔓`,
		`cert_key_password: "********"`)
}

func TestSNMPConfig(t *testing.T) {
	assertClean(t,
		`community_string: password`,
		`community_string: "********"`)
	assertClean(t,
		`authKey: password`,
		`authKey: "********"`)
	assertClean(t,
		`authkey: password`,
		`authkey: "********"`)
	assertClean(t,
		`auth_key: password`,
		`auth_key: "********"`)
	assertClean(t,
		`privKey: password`,
		`privKey: "********"`)
	assertClean(t,
		`privkey: password`,
		`privkey: "********"`)
	assertClean(t,
		`priv_key: password`,
		`priv_key: "********"`)
	assertClean(t,
		`community_string: p@ssw0r)`,
		`community_string: "********"`)
	assertClean(t,
		`community_string: 🔑 🔒 🔐 🔓`,
		`community_string: "********"`)
	assertClean(t,
		`community_string: password`,
		`community_string: "********"`)
	assertClean(t,
		`community_string: p@ssw0r)`,
		`community_string: "********"`)
	assertClean(t,
		`community_string: "password"`,
		`community_string: "********"`)
	assertClean(t,
		`community_string: 'password'`,
		`community_string: "********"`)
	assertClean(t,
		`   community_string:   'password'   `,
		`   community_string: "********"`)
	assertClean(t,
		`
network_devices:
  snmp_traps:
    community_strings:
		- 'password1'
		- 'password2'
other_config: 1
other_config_with_list: [abc]
`,
		`
network_devices:
  snmp_traps:
    community_strings: "********"
other_config: 1
other_config_with_list: [abc]
`)
	assertClean(t,
		`
network_devices:
  snmp_traps:
    community_strings: ['password1', 'password2']
other_config: 1
other_config_with_list: [abc]
`,
		`
network_devices:
  snmp_traps:
    community_strings: "********"
other_config: 1
other_config_with_list: [abc]
`)
	assertClean(t,
		`
network_devices:
  snmp_traps:
    community_strings: []
other_config: 1
other_config_with_list: [abc]
`,
		`
network_devices:
  snmp_traps:
    community_strings: "********"
other_config: 1
other_config_with_list: [abc]
`)
	assertClean(t,
		`
network_devices:
  snmp_traps:
    community_strings: [
   'password1',
   'password2']
other_config: 1
other_config_with_list: [abc]
`,
		`
network_devices:
  snmp_traps:
    community_strings: "********"
other_config: 1
other_config_with_list: [abc]
`)
	assertClean(t,
		`
snmp_traps_config:
  community_strings:
  - 'password1'
  - 'password2'
other_config: 1
other_config_with_list: [abc]
`,
		`snmp_traps_config:
  community_strings: "********"
other_config: 1
other_config_with_list: [abc]
`)
	assertClean(t,
		`community: password`,
		`community: "********"`)
	assertClean(t,
		`authentication_key: password`,
		`authentication_key: "********"`)
	assertClean(t,
		`privacy_key: password`,
		`privacy_key: "********"`)
}

func TestAddStrippedKeys(t *testing.T) {
	contents := `foobar: baz`
	cleaned, err := ScrubBytes([]byte(contents))
	require.NoError(t, err)

	// Sanity check
	assert.Equal(t, contents, string(cleaned))

	AddStrippedKeys([]string{"foobar"})

	assertClean(t, contents, `foobar: "********"`)

	dynamicReplacers = []Replacer{}
}

func TestAddStrippedKeysNewReplacer(t *testing.T) {
	contents := `foobar: baz`
	AddStrippedKeys([]string{"foobar"})

	newScrubber := New()
	AddDefaultReplacers(newScrubber)

	cleaned, err := newScrubber.ScrubBytes([]byte(contents))
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(`foobar: "********"`), strings.TrimSpace(string(cleaned)))

	dynamicReplacers = []Replacer{}
}

func TestCertConfig(t *testing.T) {
	assertClean(t,
		`cert_key: >
		   -----BEGIN PRIVATE KEY-----
		   MIICdQIBADANBgkqhkiG9w0BAQEFAASCAl8wggJbAgEAAoGBAOLJKRals8tGoy7K
		   ljG6/hMcoe16W6MPn47Q601ttoFkMoSJZ1Jos6nxn32KXfG6hCiB0bmf1iyZtaMa
		   idae/ceT7ZNGvqcVffpDianq9r08hClhnU8mTojl38fsvHf//yqZNzn1ZUcLsY9e
		   wG6wl7CsbWCafxaw+PfaCB1uWlnhAgMBAAECgYAI+tQgrHEBFIvzl1v5HiFfWlvj
		   DlxAiabUvdsDVtvKJdCGRPaNYc3zZbjd/LOZlbwT6ogGZJjTbUau7acVk3gS8uKl
		   ydWWODSuxVYxY8Poxt9SIksOAk5WmtMgIg2bTltTb8z3AWAT3qZrHth03la5Zbix
		   ynEngzyj1+ND7YwQAQJBAP00t8/1aqub+rfza+Ddd8OYSMARFH22oxgy2W1O+Gwc
		   Y8Gn3z6TkadfhPxFaUPnBPx8wm3mN+XeSB1nf0KCAWECQQDlSc7jQ/Ps5rxcoekB
		   ldB+VmuR8TfcWdrWSOdHUiLyoJoj+Z7yfrf70gONPP9tUnwX6MYdT8YwzHK34aWv
		   8KiBAkBHddlql5jDVgIsaEbJ77cdPJ1Ll4Zw9FqTOcajUuZJnLmKrhYTUxKIaize
		   BbjvsQN3Pr6gxZiBB3rS0aLY4lgBAkApsH3ZfKWBUYK2JQpEq4S5M+VjJ8TMX9oW
		   VDMZGKoaC3F7UQvBc6DoPItAxvJ6YiEGB+Ddu3+Bp+rD3FdP4iYBAkBh17O56A/f
		   QX49RjRCRIT0w4nvZ3ph9gHEe50E4+Ky5CLQNOPLD/RbBXSEzez8cGysVvzDO3DZ
		   /iN4a8gloY3d
		   -----END PRIVATE KEY-----`,
		`cert_key: >
		   ********`)
	assertClean(t,
		`cert_key: |
			-----BEGIN CERTIFICATE-----
			MIICdQIBADANBgkqhkiG9w0BAQEFAASCAl8wggJbAgEAAoGBAOLJKRals8tGoy7K
			ljG6/hMcoe16W6MPn47Q601ttoFkMoSJZ1Jos6nxn32KXfG6hCiB0bmf1iyZtaMa
			idae/ceT7ZNGvqcVffpDianq9r08hClhnU8mTojl38fsvHf//yqZNzn1ZUcLsY9e
			wG6wl7CsbWCafxaw+PfaCB1uWlnhAgMBAAECgYAI+tQgrHEBFIvzl1v5HiFfWlvj
			DlxAiabUvdsDVtvKJdCGRPaNYc3zZbjd/LOZlbwT6ogGZJjTbUau7acVk3gS8uKl
			ydWWODSuxVYxY8Poxt9SIksOAk5WmtMgIg2bTltTb8z3AWAT3qZrHth03la5Zbix
			ynEngzyj1+ND7YwQAQJBAP00t8/1aqub+rfza+Ddd8OYSMARFH22oxgy2W1O+Gwc
			Y8Gn3z6TkadfhPxFaUPnBPx8wm3mN+XeSB1nf0KCAWECQQDlSc7jQ/Ps5rxcoekB
			ldB+VmuR8TfcWdrWSOdHUiLyoJoj+Z7yfrf70gONPP9tUnwX6MYdT8YwzHK34aWv
			8KiBAkBHddlql5jDVgIsaEbJ77cdPJ1Ll4Zw9FqTOcajUuZJnLmKrhYTUxKIaize
			BbjvsQN3Pr6gxZiBB3rS0aLY4lgBAkApsH3ZfKWBUYK2JQpEq4S5M+VjJ8TMX9oW
			VDMZGKoaC3F7UQvBc6DoPItAxvJ6YiEGB+Ddu3+Bp+rD3FdP4iYBAkBh17O56A/f
			QX49RjRCRIT0w4nvZ3ph9gHEe50E4+Ky5CLQNOPLD/RbBXSEzez8cGysVvzDO3DZ
			/iN4a8gloY3d
			-----END CERTIFICATE-----`,
		`cert_key: |
			********`)
}

func TestConfig(t *testing.T) {
	assertClean(t,
		`dd_url: https://app.datadoghq.com
api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
proxy: http://user:password@host:port
password: foo
auth_token: bar
auth_token_file_path: /foo/bar/baz
kubelet_auth_token_path: /foo/bar/kube_token
# comment to strip
network_devices:
  snmp_traps:
    community_strings:
    - 'password1'
    - 'password2'
log_level: info`,
		`dd_url: https://app.datadoghq.com
api_key: "***************************aaaaa"
proxy: http://user:********@host:port
password: "********"
auth_token: "********"
auth_token_file_path: /foo/bar/baz
kubelet_auth_token_path: /foo/bar/kube_token
network_devices:
  snmp_traps:
    community_strings: "********"
log_level: info`)
}

func TestBearerToken(t *testing.T) {
	assertClean(t,
		`Bearer 2fe663014abcd1850076f6d68c0355666db98758262870811cace007cd4a62ba`,
		`Bearer ***********************************************************a62ba`)
	assertClean(t,
		`Error: Get "https://localhost:5001/agent/status": net/http: invalid header field value "Bearer 260a9c065b6426f81b7abae9e6bca9a16f7a842af65c940e89e3417c7aaec82d\n\n" for key Authorization`,
		`Error: Get "https://localhost:5001/agent/status": net/http: invalid header field value "Bearer ***********************************************************ec82d\n\n" for key Authorization`)
	// entirely clean token with different length that 64
	assertClean(t,
		`Bearer 2fe663014abcd18`,
		`Bearer ********`)
	assertClean(t,
		`Bearer 2fe663014abcd1850076f6d68c0355666db98758262870811cace007cd4a62bdsldijfoiwjeoimdfolisdjoijfewoa`,
		`Bearer ********`)
	assertClean(t,
		`Bearer abf243d1-9ba5-4d8d-8365-ac18229eb2ac`,
		`Bearer ********`)
	assertClean(t,
		`Bearer token with space`,
		`Bearer ********`)
	assertClean(t,
		`Bearer     123456798`,
		`Bearer ********`)
	assertClean(t,
		`AuthBearer 2fe663014abcd1850076f6d68c0355666db98758262870811cace007cd4a62ba`,
		`AuthBearer 2fe663014abcd1850076f6d68c0355666db98758262870811cace007cd4a62ba`)
}

func TestAuthorization(t *testing.T) {
	assertClean(t,
		`Authorization: some auth`,
		`Authorization: "********"`)
	assertClean(t,
		`  Authorization: some auth`,
		`  Authorization: "********"`)
	assertClean(t,
		`- Authorization: some auth`,
		`- Authorization: "********"`)
	assertClean(t,
		`  authorization: some auth`,
		`  authorization: "********"`)
}

func TestOAuthCredentials(t *testing.T) {
	// Test consumer_key
	assertClean(t,
		`consumer_key: my_consumer_key_123`,
		`consumer_key: "********"`)
	assertClean(t,
		`  consumer_key: "my_consumer_key_123"`,
		`  consumer_key: "********"`)

	// Test consumer_secret
	assertClean(t,
		`consumer_secret: my_consumer_secret_456`,
		`consumer_secret: "********"`)
	assertClean(t,
		`  consumer_secret: 'my_consumer_secret_456'`,
		`  consumer_secret: "********"`)

	// Test token_id
	assertClean(t,
		`token_id: my_token_id_789`,
		`token_id: "********"`)
	assertClean(t,
		`  token_id: "my_token_id_789"`,
		`  token_id: "********"`)

	// Test token_secret
	assertClean(t,
		`token_secret: my_token_secret_abc`,
		`token_secret: "********"`)
	assertClean(t,
		`  token_secret: 'my_token_secret_abc'`,
		`  token_secret: "********"`)

	// Test mixed OAuth configuration
	assertClean(t,
		`oauth_config:
  consumer_key: my_consumer_key
  consumer_secret: my_consumer_secret
  token_id: my_token_id
  token_secret: my_token_secret`,
		`oauth_config:
  consumer_key: "********"
  consumer_secret: "********"
  token_id: "********"
  token_secret: "********"`)
}

func TestScrubCommandsEnv(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"api key",
			`DD_API_KEY=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa agent run`,
			`DD_API_KEY=***************************aaaaa agent run`,
		}, {
			"app key",
			`DD_APP_KEY=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa agent run`,
			`DD_APP_KEY=***********************************aaaaa agent run`,
		},
	}

	for _, tc := range testCases {
		t.Run("line "+tc.name, func(t *testing.T) {
			scrubbed := ScrubLine(tc.input)
			assert.EqualValues(t, tc.expected, scrubbed)
		})
		t.Run("bytes "+tc.name, func(t *testing.T) {
			scrubbed, err := ScrubBytes([]byte(tc.input))
			require.NoError(t, err)
			assert.EqualValues(t, tc.expected, scrubbed)
		})
	}
}

func TestSecretConfigurationVariablesNotScrubbed(t *testing.T) {
	// Test that secret-related configuration variables are NOT scrubbed
	// These should remain unchanged in the output
	secretConfigTests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"secret_name",
			`secret_name: "my-secret-name"`,
			`secret_name: "my-secret-name"`,
		},
		{
			"secret_audit_file_max_size",
			`secret_audit_file_max_size: 1048576`,
			`secret_audit_file_max_size: 1048576`,
		},
		{
			"secret_backend_arguments",
			`secret_backend_arguments: ["arg1", "arg2"]`,
			`secret_backend_arguments: ["arg1", "arg2"]`,
		},
		{
			"secret_backend_command",
			`secret_backend_command: "/usr/local/bin/secret-helper"`,
			`secret_backend_command: "/usr/local/bin/secret-helper"`,
		},
		{
			"secret_backend_command_allow_group_exec_perm",
			`secret_backend_command_allow_group_exec_perm: true`,
			`secret_backend_command_allow_group_exec_perm: true`,
		},
		{
			"secret_backend_config",
			`secret_backend_config: {"key": "value"}`,
			`secret_backend_config: {"key": "value"}`,
		},
		{
			"secret_backend_output_max_size",
			`secret_backend_output_max_size: 1024`,
			`secret_backend_output_max_size: 1024`,
		},
		{
			"secret_backend_remove_trailing_line_break",
			`secret_backend_remove_trailing_line_break: false`,
			`secret_backend_remove_trailing_line_break: false`,
		},
		{
			"secret_backend_skip_checks",
			`secret_backend_skip_checks: true`,
			`secret_backend_skip_checks: true`,
		},
		{
			"secret_backend_timeout",
			`secret_backend_timeout: 30`,
			`secret_backend_timeout: 30`,
		},
		{
			"secret_backend_type",
			`secret_backend_type: "vault"`,
			`secret_backend_type: "vault"`,
		},
		{
			"secret_refresh_interval",
			`secret_refresh_interval: 3600`,
			`secret_refresh_interval: 3600`,
		},
		{
			"secret_refresh_scatter",
			`secret_refresh_scatter: true`,
			`secret_refresh_scatter: true`,
		},
		{
			"admission_controller.certificate.secret_name",
			`admission_controller:
  certificate:
    secret_name: "webhook-certificate"`,
			`admission_controller:
  certificate:
    secret_name: "webhook-certificate"`,
		},
		{
			"mixed secret configuration with sensitive data",
			`secret_backend_type: "vault"
secret_backend_command: "/usr/local/bin/vault-helper"
secret_backend_arguments: ["--config", "/etc/vault.conf"]
secret_backend_timeout: 30
secret_refresh_interval: 3600
api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
password: "sensitive_password"
other_config: "not_secret"`,
			`secret_backend_type: "vault"
secret_backend_command: "/usr/local/bin/vault-helper"
secret_backend_arguments: ["--config", "/etc/vault.conf"]
secret_backend_timeout: 30
secret_refresh_interval: 3600
api_key: "***************************aaaaa"
password: "********"
other_config: "not_secret"`,
		},
	}

	for _, tc := range secretConfigTests {
		t.Run(tc.name, func(t *testing.T) {
			assertClean(t, tc.input, tc.expected)
		})
	}
}

func TestConfigFile(t *testing.T) {
	cleanedConfigFile := `dd_url: https://app.datadoghq.com

api_key: "***************************aaaaa"

proxy: http://user:********@host:port






dogstatsd_port : 8125


log_level: info
`

	wd, _ := os.Getwd()
	filePath := filepath.Join(wd, "test", "datadog.yaml")
	cleaned, err := ScrubFile(filePath)
	assert.NoError(t, err)
	cleanedString := string(cleaned)

	assert.Equal(t, cleanedConfigFile, cleanedString)
}

func TestNewHTTPHeaderAndExactKeys(t *testing.T) {
	// Test HTTP header-style API keys with "key" suffix
	assertClean(t,
		`x-api-key: abc123def456`,
		`x-api-key: "********"`)
	assertClean(t,
		`x-dreamfactory-api-key: secret123`,
		`x-dreamfactory-api-key: "********"`)
	assertClean(t,
		`x-functions-key: mykey456`,
		`x-functions-key: "********"`)
	assertClean(t,
		`x-lz-api-key: lzkey789`,
		`x-lz-api-key: "********"`)
	assertClean(t,
		`x-octopus-apikey: octopuskey`,
		`x-octopus-apikey: "********"`)
	assertClean(t,
		`x-pm-partner-key: partnerkey123`,
		`x-pm-partner-key: "********"`)
	assertClean(t,
		`x-rapidapi-key: rapidkey456`,
		`x-rapidapi-key: "********"`)
	assertClean(t,
		`x-sungard-idp-api-key: sungardkey`,
		`x-sungard-idp-api-key: "********"`)
	assertClean(t,
		`x-vtex-api-appkey: vtexkey789`,
		`x-vtex-api-appkey: "********"`)
	assertClean(t,
		`x-seel-api-key: seelkey123`,
		`x-seel-api-key: "********"`)
	assertClean(t,
		`x-goog-api-key: googlekey456`,
		`x-goog-api-key: "********"`)
	assertClean(t,
		`x-sonar-passcode: sonarpass789`,
		`x-sonar-passcode: "********"`)

	// Test HTTP header-style API keys with "token" suffix
	assertClean(t,
		`x-auth-token: authtoken123`,
		`x-auth-token: "********"`)
	assertClean(t,
		`x-rundeck-auth-token: rundecktoken`,
		`x-rundeck-auth-token: "********"`)
	assertClean(t,
		`x-consul-token: consultoken123`,
		`x-consul-token: "********"`)
	assertClean(t,
		`x-datadog-monitor-token: ddmonitortoken`,
		`x-datadog-monitor-token: "********"`)
	assertClean(t,
		`x-vault-token: vaulttoken456`,
		`x-vault-token: "********"`)
	assertClean(t,
		`x-vtex-api-apptoken: vtexapptoken`,
		`x-vtex-api-apptoken: "********"`)
	assertClean(t,
		`x-static-token: statictoken789`,
		`x-static-token: "********"`)

	// Test HTTP header-style API keys with "auth" suffix
	assertClean(t,
		`x-auth: authvalue123`,
		`x-auth: "********"`)
	assertClean(t,
		`x-stratum-auth: stratumauth`,
		`x-stratum-auth: "********"`)

	// Test HTTP header-style API keys with "secret" suffix
	assertClean(t,
		`x-api-secret: apisecret123`,
		`x-api-secret: "********"`)
	assertClean(t,
		`x-ibm-client-secret: ibmsecret456`,
		`x-ibm-client-secret: "********"`)
	assertClean(t,
		`x-chalk-client-secret: chalksecret789`,
		`x-chalk-client-secret: "********"`)

	// Test exact key matches
	assertClean(t,
		`auth-tenantid: tenant123`,
		`auth-tenantid: "********"`)
	assertClean(t,
		`authority: auth123`,
		`authority: "********"`)
	assertClean(t,
		`cainzapp-api-key: cainzkey456`,
		`cainzapp-api-key: "********"`)
	assertClean(t,
		`cms-svc-api-key: cmskey789`,
		`cms-svc-api-key: "********"`)
	assertClean(t,
		`lodauth: lodauth123`,
		`lodauth: "********"`)
	assertClean(t,
		`sec-websocket-key: websocketkey`,
		`sec-websocket-key: "********"`)
	assertClean(t,
		`statuskey: status123`,
		`statuskey: "********"`)
	assertClean(t,
		`cookie: cookievalue123`,
		`cookie: "********"`)
	assertClean(t,
		`private-token: privatetoken456`,
		`private-token: "********"`)
	assertClean(t,
		`kong-admin-token: kongadmintoken`,
		`kong-admin-token: "********"`)
	assertClean(t,
		`accesstoken: accesstoken789`,
		`accesstoken: "********"`)
	assertClean(t,
		`session_token: sessiontoken123`,
		`session_token: "********"`)

	// Test that non-matching keys are not scrubbed
	assertClean(t,
		`regular_key: should_not_be_scrubbed`,
		`regular_key: should_not_be_scrubbed`)
	assertClean(t,
		`some-other-key: also_not_scrubbed`,
		`some-other-key: also_not_scrubbed`)
}

func TestPrivateActionRunnerPrivateKey(t *testing.T) {
	// Test private action runner key configuration
	assertClean(t,
		`private_key: abc123def456`,
		`private_key: "********"`)
}
