create table t(n number);
grant select,insert on t to c##datadog ;
insert into t values(18446744073709551615);
