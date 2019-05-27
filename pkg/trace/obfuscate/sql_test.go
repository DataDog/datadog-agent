package obfuscate

import (
	"strconv"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
)

type sqlTestCase struct {
	query    string
	expected string
}

func SQLSpan(query string) *pb.Span {
	return &pb.Span{
		Resource: query,
		Type:     "sql",
		Meta: map[string]string{
			"sql.query": query,
		},
	}
}

func TestSQLResourceQuery(t *testing.T) {
	assert := assert.New(t)
	span := &pb.Span{
		Resource: "SELECT * FROM users WHERE id = 42",
		Type:     "sql",
		Meta: map[string]string{
			"sql.query": "SELECT * FROM users WHERE id = 42",
		},
	}

	NewObfuscator(nil).Obfuscate(span)
	assert.Equal("SELECT * FROM users WHERE id = ?", span.Resource)
	assert.Equal("SELECT * FROM users WHERE id = 42", span.Meta["sql.query"])
}

func TestSQLResourceWithoutQuery(t *testing.T) {
	assert := assert.New(t)
	span := &pb.Span{
		Resource: "SELECT * FROM users WHERE id = 42",
		Type:     "sql",
	}

	NewObfuscator(nil).Obfuscate(span)
	assert.Equal("SELECT * FROM users WHERE id = ?", span.Resource)
	assert.Equal("SELECT * FROM users WHERE id = ?", span.Meta["sql.query"])
}

func TestSQLResourceWithError(t *testing.T) {
	assert := assert.New(t)
	testCases := []struct {
		span pb.Span
	}{
		{
			pb.Span{
				Resource: "SELECT * FROM users WHERE id = '' AND '",
				Type:     "sql",
			},
		},
		{
			pb.Span{
				Resource: "INSERT INTO pages (id, name) VALUES (%(id0)s, %(name0)s), (%(id1)s, %(name1",
				Type:     "sql",
			},
		},
		{
			pb.Span{
				Resource: "INSERT INTO pages (id, name) VALUES (%(id0)s, %(name0)s), (%(id1)s, %(name1)",
				Type:     "sql",
			},
		},
	}

	for _, tc := range testCases {
		// copy test cases as Quantize mutates
		testSpan := tc.span

		NewObfuscator(nil).Obfuscate(&tc.span)
		assert.Equal("Non-parsable SQL query", tc.span.Resource)
		assert.Equal(testSpan.Resource, tc.span.Meta["sql.query"])
	}
}

func TestSQLQuantizer(t *testing.T) {
	cases := []sqlTestCase{
		{
			"select * from users where id = 42",
			"select * from users where id = ?",
		},
		{
			"SELECT host, status FROM ec2_status WHERE org_id = 42",
			"SELECT host, status FROM ec2_status WHERE org_id = ?",
		},
		{
			"SELECT host, status FROM ec2_status WHERE org_id=42",
			"SELECT host, status FROM ec2_status WHERE org_id = ?",
		},
		{
			"-- get user \n--\n select * \n   from users \n    where\n       id = 214325346",
			"select * from users where id = ?",
		},
		{
			"SELECT * FROM `host` WHERE `id` IN (42, 43) /*comment with parameters,host:localhost,url:controller#home,id:FF005:00CAA*/",
			"SELECT * FROM host WHERE id IN ( ? )",
		},
		{
			"SELECT `host`.`address` FROM `host` WHERE org_id=42",
			"SELECT host . address FROM host WHERE org_id = ?",
		},
		{
			`SELECT "host"."address" FROM "host" WHERE org_id=42`,
			`SELECT host . address FROM host WHERE org_id = ?`,
		},
		{
			`SELECT * FROM host WHERE id IN (42, 43) /*
			multiline comment with parameters,
			host:localhost,url:controller#home,id:FF005:00CAA
			*/`,
			"SELECT * FROM host WHERE id IN ( ? )",
		},
		{
			"UPDATE user_dash_pref SET json_prefs = %(json_prefs)s, modified = '2015-08-27 22:10:32.492912' WHERE user_id = %(user_id)s AND url = %(url)s",
			"UPDATE user_dash_pref SET json_prefs = ? modified = ? WHERE user_id = ? AND url = ?"},
		{
			"SELECT DISTINCT host.id AS host_id FROM host JOIN host_alias ON host_alias.host_id = host.id WHERE host.org_id = %(org_id_1)s AND host.name NOT IN (%(name_1)s) AND host.name IN (%(name_2)s, %(name_3)s, %(name_4)s, %(name_5)s)",
			"SELECT DISTINCT host.id FROM host JOIN host_alias ON host_alias.host_id = host.id WHERE host.org_id = ? AND host.name NOT IN ( ? ) AND host.name IN ( ? )",
		},
		{
			"SELECT org_id, metric_key FROM metrics_metadata WHERE org_id = %(org_id)s AND metric_key = ANY(array[75])",
			"SELECT org_id, metric_key FROM metrics_metadata WHERE org_id = ? AND metric_key = ANY ( array [ ? ] )",
		},
		{
			"SELECT org_id, metric_key   FROM metrics_metadata   WHERE org_id = %(org_id)s AND metric_key = ANY(array[21, 25, 32])",
			"SELECT org_id, metric_key FROM metrics_metadata WHERE org_id = ? AND metric_key = ANY ( array [ ? ] )",
		},
		{
			"SELECT articles.* FROM articles WHERE articles.id = 1 LIMIT 1",
			"SELECT articles.* FROM articles WHERE articles.id = ? LIMIT ?",
		},

		{
			"SELECT articles.* FROM articles WHERE articles.id = 1 LIMIT 1, 20",
			"SELECT articles.* FROM articles WHERE articles.id = ? LIMIT ?",
		},
		{
			"SELECT articles.* FROM articles WHERE articles.id = 1 LIMIT 1, 20;",
			"SELECT articles.* FROM articles WHERE articles.id = ? LIMIT ?",
		},
		{
			"SELECT articles.* FROM articles WHERE articles.id = 1 LIMIT 15,20;",
			"SELECT articles.* FROM articles WHERE articles.id = ? LIMIT ?",
		},
		{
			"SELECT articles.* FROM articles WHERE articles.id = 1 LIMIT 1;",
			"SELECT articles.* FROM articles WHERE articles.id = ? LIMIT ?",
		},
		{
			"SELECT articles.* FROM articles WHERE (articles.created_at BETWEEN '2016-10-31 23:00:00.000000' AND '2016-11-01 23:00:00.000000')",
			"SELECT articles.* FROM articles WHERE ( articles.created_at BETWEEN ? AND ? )",
		},
		{
			"SELECT articles.* FROM articles WHERE (articles.created_at BETWEEN $1 AND $2)",
			"SELECT articles.* FROM articles WHERE ( articles.created_at BETWEEN ? AND ? )",
		},
		{
			"SELECT articles.* FROM articles WHERE (articles.published != true)",
			"SELECT articles.* FROM articles WHERE ( articles.published != ? )",
		},
		{
			"SELECT articles.* FROM articles WHERE (title = 'guides.rubyonrails.org')",
			"SELECT articles.* FROM articles WHERE ( title = ? )",
		},
		{
			"SELECT articles.* FROM articles WHERE ( title = ? ) AND ( author = ? )",
			"SELECT articles.* FROM articles WHERE ( title = ? ) AND ( author = ? )",
		},
		{
			"SELECT articles.* FROM articles WHERE ( title = :title )",
			"SELECT articles.* FROM articles WHERE ( title = :title )",
		},
		{
			"SELECT articles.* FROM articles WHERE ( title = @title )",
			"SELECT articles.* FROM articles WHERE ( title = @title )",
		},
		{
			"SELECT date(created_at) as ordered_date, sum(price) as total_price FROM orders GROUP BY date(created_at) HAVING sum(price) > 100",
			"SELECT date ( created_at ), sum ( price ) FROM orders GROUP BY date ( created_at ) HAVING sum ( price ) > ?",
		},
		{
			"SELECT * FROM articles WHERE id > 10 ORDER BY id asc LIMIT 20",
			"SELECT * FROM articles WHERE id > ? ORDER BY id asc LIMIT ?",
		},
		{
			"SELECT clients.* FROM clients INNER JOIN posts ON posts.author_id = author.id AND posts.published = 't'",
			"SELECT clients.* FROM clients INNER JOIN posts ON posts.author_id = author.id AND posts.published = ?",
		},
		{
			"SELECT articles.* FROM articles WHERE articles.id IN (1, 3, 5)",
			"SELECT articles.* FROM articles WHERE articles.id IN ( ? )",
		},
		{
			"SELECT * FROM clients WHERE (clients.first_name = 'Andy') LIMIT 1 BEGIN INSERT INTO clients (created_at, first_name, locked, orders_count, updated_at) VALUES ('2011-08-30 05:22:57', 'Andy', 1, NULL, '2011-08-30 05:22:57') COMMIT",
			"SELECT * FROM clients WHERE ( clients.first_name = ? ) LIMIT ? BEGIN INSERT INTO clients ( created_at, first_name, locked, orders_count, updated_at ) VALUES ( ? ) COMMIT",
		},
		{
			"SELECT * FROM clients WHERE (clients.first_name = 'Andy') LIMIT 15, 25 BEGIN INSERT INTO clients (created_at, first_name, locked, orders_count, updated_at) VALUES ('2011-08-30 05:22:57', 'Andy', 1, NULL, '2011-08-30 05:22:57') COMMIT",
			"SELECT * FROM clients WHERE ( clients.first_name = ? ) LIMIT ? BEGIN INSERT INTO clients ( created_at, first_name, locked, orders_count, updated_at ) VALUES ( ? ) COMMIT",
		},
		{
			"SAVEPOINT \"s139956586256192_x1\"",
			"SAVEPOINT ?",
		},
		{
			"INSERT INTO user (id, username) VALUES ('Fred','Smith'), ('John','Smith'), ('Michael','Smith'), ('Robert','Smith');",
			"INSERT INTO user ( id, username ) VALUES ( ? )",
		},
		{
			"CREATE KEYSPACE Excelsior WITH replication = {'class': 'SimpleStrategy', 'replication_factor' : 3};",
			"CREATE KEYSPACE Excelsior WITH replication = ?",
		},
		{
			`SELECT "webcore_page"."id" FROM "webcore_page" WHERE "webcore_page"."slug" = %s ORDER BY "webcore_page"."path" ASC LIMIT 1`,
			"SELECT webcore_page . id FROM webcore_page WHERE webcore_page . slug = ? ORDER BY webcore_page . path ASC LIMIT ?",
		},
		{
			"SELECT server_table.host AS host_id FROM table#.host_tags as server_table WHERE server_table.host_id = 50",
			"SELECT server_table.host FROM table#.host_tags WHERE server_table.host_id = ?",
		},
		{
			`INSERT INTO delayed_jobs (attempts, created_at, failed_at, handler, last_error, locked_at, locked_by, priority, queue, run_at, updated_at) VALUES (0, '2016-12-04 17:09:59', NULL, '--- !ruby/object:Delayed::PerformableMethod\nobject: !ruby/object:Item\n  store:\n  - a simple string\n  - an \'escaped \' string\n  - another \'escaped\' string\n  - 42\n  string: a string with many \\\\\'escapes\\\\\'\nmethod_name: :show_store\nargs: []\n', NULL, NULL, NULL, 0, NULL, '2016-12-04 17:09:59', '2016-12-04 17:09:59')`,
			"INSERT INTO delayed_jobs ( attempts, created_at, failed_at, handler, last_error, locked_at, locked_by, priority, queue, run_at, updated_at ) VALUES ( ? )",
		},
		{
			"SELECT name, pretty_print(address) FROM people;",
			"SELECT name, pretty_print ( address ) FROM people",
		},
		{
			"* SELECT * FROM fake_data(1, 2, 3);",
			"* SELECT * FROM fake_data ( ? )",
		},
		{
			"CREATE FUNCTION add(integer, integer) RETURNS integer\n AS 'select $1 + $2;'\n LANGUAGE SQL\n IMMUTABLE\n RETURNS NULL ON NULL INPUT;",
			"CREATE FUNCTION add ( integer, integer ) RETURNS integer LANGUAGE SQL IMMUTABLE RETURNS ? ON ? INPUT",
		},
		{
			"SELECT * FROM public.table ( array [ ROW ( array [ 'magic', 'foo',",
			"SELECT * FROM public.table ( array [ ROW ( array [ ?",
		},
		{
			"SELECT pg_try_advisory_lock (123) AS t46eef3f025cc27feb31ca5a2d668a09a",
			"SELECT pg_try_advisory_lock ( ? )",
		},
		{
			"INSERT INTO `qual-aa`.issues (alert0 , alert1) VALUES (NULL, NULL)",
			"INSERT INTO qual-aa . issues ( alert0, alert1 ) VALUES ( ? )",
		},
		{
			"INSERT INTO user (id, email, name) VALUES (null, ?, ?)",
			"INSERT INTO user ( id, email, name ) VALUES ( ? )",
		},
		{
			"select * from users where id = 214325346     # This comment continues to the end of line",
			"select * from users where id = ?",
		},
		{
			"select * from users where id = 214325346     -- This comment continues to the end of line",
			"select * from users where id = ?",
		},
		{
			"SELECT * FROM /* this is an in-line comment */ users;",
			"SELECT * FROM users",
		},
		{
			"SELECT /*! STRAIGHT_JOIN */ col1 FROM table1",
			"SELECT col1 FROM table1",
		},
		{
			`DELETE FROM t1
			WHERE s11 > ANY
			(SELECT COUNT(*) /* no hint */ FROM t2
			WHERE NOT EXISTS
			(SELECT * FROM t3
			WHERE ROW(5*t2.s1,77)=
			(SELECT 50,11*s1 FROM t4 UNION SELECT 50,77 FROM
			(SELECT * FROM t5) AS t5)));`,
			"DELETE FROM t1 WHERE s11 > ANY ( SELECT COUNT ( * ) FROM t2 WHERE NOT EXISTS ( SELECT * FROM t3 WHERE ROW ( ? * t2.s1, ? ) = ( SELECT ? * s1 FROM t4 UNION SELECT ? FROM ( SELECT * FROM t5 ) ) ) )",
		},
		{
			"SET @g = 'POLYGON((0 0,10 0,10 10,0 10,0 0),(5 5,7 5,7 7,5 7, 5 5))';",
			"SET @g = ?",
		},
		{
			`SELECT daily_values.*,
                    LEAST((5040000 - @runtot), value) AS value,
                    ` + "(@runtot := @runtot + daily_values.value) AS total FROM (SELECT @runtot:=0) AS n, `daily_values`  WHERE `daily_values`.`subject_id` = 12345 AND `daily_values`.`subject_type` = 'Skippity' AND (daily_values.date BETWEEN '2018-05-09' AND '2018-06-19') HAVING value >= 0 ORDER BY date",
			`SELECT daily_values.*, LEAST ( ( ? - @runtot ), value ), ( @runtot := @runtot + daily_values.value ) FROM ( SELECT @runtot := ? ), daily_values WHERE daily_values . subject_id = ? AND daily_values . subject_type = ? AND ( daily_values.date BETWEEN ? AND ? ) HAVING value >= ? ORDER BY date`,
		},
		{
			`    SELECT
      t1.userid,
      t1.fullname,
      t1.firm_id,
      t2.firmname,
      t1.email,
      t1.location,
      t1.state,
      t1.phone,
      t1.url,
      DATE_FORMAT( t1.lastmod, "%m/%d/%Y %h:%i:%s" ) AS lastmod,
      t1.lastmod AS lastmod_raw,
      t1.user_status,
      t1.pw_expire,
      DATE_FORMAT( t1.pw_expire, "%m/%d/%Y" ) AS pw_expire_date,
      t1.addr1,
      t1.addr2,
      t1.zipcode,
      t1.office_id,
      t1.default_group,
      t3.firm_status,
      t1.title
    FROM
           userdata      AS t1
      LEFT JOIN lawfirm_names AS t2 ON t1.firm_id = t2.firm_id
      LEFT JOIN lawfirms      AS t3 ON t1.firm_id = t3.firm_id
    WHERE
      t1.userid = 'jstein'

  `,
			`SELECT t1.userid, t1.fullname, t1.firm_id, t2.firmname, t1.email, t1.location, t1.state, t1.phone, t1.url, DATE_FORMAT ( t1.lastmod, %m/%d/%Y %h:%i:%s ), t1.lastmod, t1.user_status, t1.pw_expire, DATE_FORMAT ( t1.pw_expire, %m/%d/%Y ), t1.addr1, t1.addr2, t1.zipcode, t1.office_id, t1.default_group, t3.firm_status, t1.title FROM userdata LEFT JOIN lawfirm_names ON t1.firm_id = t2.firm_id LEFT JOIN lawfirms ON t1.firm_id = t3.firm_id WHERE t1.userid = ?`,
		},
		{
			`SELECT [b].[BlogId], [b].[Name]
FROM [Blogs] AS [b]
ORDER BY [b].[Name]`,
			`SELECT [ b ] . [ BlogId ], [ b ] . [ Name ] FROM [ Blogs ] ORDER BY [ b ] . [ Name ]`,
		},
		{
			`SELECT * FROM users WHERE firstname=''`,
			`SELECT * FROM users WHERE firstname = ?`,
		},
		{
			`SELECT * FROM users WHERE firstname=' '`,
			`SELECT * FROM users WHERE firstname = ?`,
		},
		{
			`SELECT * FROM users WHERE firstname=""`,
			`SELECT * FROM users WHERE firstname = ""`,
		},
		{
			`SELECT * FROM users WHERE lastname=" "`,
			`SELECT * FROM users WHERE lastname = ""`,
		},
		{
			`SELECT * FROM users WHERE lastname="	 "`,
			`SELECT * FROM users WHERE lastname = ""`,
		},
		{
			`SELECT [b].[BlogId], [b].[Name]
FROM [Blogs] AS [b
ORDER BY [b].[Name]`,
			`Non-parsable SQL query`,
		},
	}

	for i, c := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			s := SQLSpan(c.query)
			NewObfuscator(nil).Obfuscate(s)
			assert.Equal(t, c.expected, s.Resource)
		})
	}
}

func TestMultipleProcess(t *testing.T) {
	assert := assert.New(t)

	testCases := []struct {
		query    string
		expected string
	}{
		{
			"SELECT clients.* FROM clients INNER JOIN posts ON posts.author_id = author.id AND posts.published = 't'",
			"SELECT clients.* FROM clients INNER JOIN posts ON posts.author_id = author.id AND posts.published = ?",
		},
		{
			"SELECT articles.* FROM articles WHERE articles.id IN (1, 3, 5)",
			"SELECT articles.* FROM articles WHERE articles.id IN ( ? )",
		},
		{
			`SELECT id FROM jq_jobs
WHERE
schedulable_at <= 1555367948 AND
queue_name = 'order_jobs' AND
status = 1 AND
id % 8 = 3
ORDER BY
schedulable_at
LIMIT 1000`,
			"SELECT id FROM jq_jobs WHERE schedulable_at <= ? AND queue_name = ? AND status = ? AND id % ? = ? ORDER BY schedulable_at LIMIT ?",
		},
	}

	// The consumer is the same between executions
	for _, tc := range testCases {
		output, err := obfuscateSQLString(tc.query)
		assert.Nil(err)
		assert.Equal(tc.expected, output)
	}
}

func TestConsumerError(t *testing.T) {
	assert := assert.New(t)

	// Malformed SQL is not accepted and the outer component knows
	// what to do with malformed SQL
	input := "SELECT * FROM users WHERE users.id = '1 AND users.name = 'dog'"

	output, err := obfuscateSQLString(input)
	assert.NotNil(err)
	assert.Equal("", output)
}

// Benchmark the Tokenizer using a SQL statement
func BenchmarkTokenizer(b *testing.B) {
	benchmarks := []struct {
		name  string
		query string
	}{
		{"Escaping", `INSERT INTO delayed_jobs (attempts, created_at, failed_at, handler, last_error, locked_at, locked_by, priority, queue, run_at, updated_at) VALUES (0, '2016-12-04 17:09:59', NULL, '--- !ruby/object:Delayed::PerformableMethod\nobject: !ruby/object:Item\n  store:\n  - a simple string\n  - an \'escaped \' string\n  - another \'escaped\' string\n  - 42\n  string: a string with many \\\\\'escapes\\\\\'\nmethod_name: :show_store\nargs: []\n', NULL, NULL, NULL, 0, NULL, '2016-12-04 17:09:59', '2016-12-04 17:09:59')`},
		{"Grouping", `INSERT INTO delayed_jobs (created_at, failed_at, handler) VALUES (0, '2016-12-04 17:09:59', NULL), (0, '2016-12-04 17:09:59', NULL), (0, '2016-12-04 17:09:59', NULL), (0, '2016-12-04 17:09:59', NULL)`},
	}
	for _, bm := range benchmarks {
		b.Run(bm.name+"/"+strconv.Itoa(len(bm.query)), func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = obfuscateSQLString(bm.query)
			}
		})
	}
}

func CassSpan(query string) *pb.Span {
	return &pb.Span{
		Resource: query,
		Type:     "cassandra",
		Meta: map[string]string{
			"query": query,
		},
	}
}

func TestCassQuantizer(t *testing.T) {
	assert := assert.New(t)

	queryToExpected := []struct{ in, expected string }{
		// List compacted and replaced
		{
			"select key, status, modified from org_check_run where org_id = %s and check in (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)",
			"select key, status, modified from org_check_run where org_id = ? and check in ( ? )",
		},
		// Some whitespace-y things
		{
			"select key, status, modified from org_check_run where org_id = %s and check in (%s, %s, %s)",
			"select key, status, modified from org_check_run where org_id = ? and check in ( ? )",
		},
		{
			"select key, status, modified from org_check_run where org_id = %s and check in (%s , %s , %s )",
			"select key, status, modified from org_check_run where org_id = ? and check in ( ? )",
		},
		// %s replaced with ? as in sql quantize
		{
			"select key, status, modified from org_check_run where org_id = %s and check = %s",
			"select key, status, modified from org_check_run where org_id = ? and check = ?",
		},
		{
			"select key, status, modified from org_check_run where org_id = %s and check = %s",
			"select key, status, modified from org_check_run where org_id = ? and check = ?",
		},
		{
			"SELECT timestamp, processes FROM process_snapshot.minutely WHERE org_id = ? AND host = ? AND timestamp >= ? AND timestamp <= ?",
			"SELECT timestamp, processes FROM process_snapshot.minutely WHERE org_id = ? AND host = ? AND timestamp >= ? AND timestamp <= ?",
		},
	}

	for _, testCase := range queryToExpected {
		s := CassSpan(testCase.in)
		NewObfuscator(nil).Obfuscate(s)
		assert.Equal(testCase.expected, s.Resource)
	}
}
