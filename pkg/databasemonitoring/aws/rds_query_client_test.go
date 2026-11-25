// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build ec2

package aws

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	awsquery "github.com/DataDog/datadog-agent/pkg/util/aws/query"
)

func TestRDSQueryClientDescribeDBInstancesAndPagination(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, http.MethodPost, r.Method)
		assert.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 "))
		assert.Contains(t, r.Header.Get("Authorization"), "/us-east-1/rds/aws4_request")
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		q, err := url.ParseQuery(string(body))
		require.NoError(t, err)
		assert.Equal(t, "DescribeDBInstances", q.Get("Action"))
		assert.Equal(t, "2014-10-31", q.Get("Version"))
		assert.Equal(t, "engine", q.Get("Filters.Filter.1.Name"))
		assert.Equal(t, "mysql", q.Get("Filters.Filter.1.Values.Value.1"))
		assert.Equal(t, "postgres", q.Get("Filters.Filter.1.Values.Value.2"))
		assert.Equal(t, "aurora-mysql", q.Get("Filters.Filter.1.Values.Value.3"))
		assert.Equal(t, "aurora-postgresql", q.Get("Filters.Filter.1.Values.Value.4"))

		w.Header().Set("Content-Type", "text/xml")
		switch q.Get("Marker") {
		case "":
			_, _ = io.WriteString(w, `<DescribeDBInstancesResponse xmlns="http://rds.amazonaws.com/doc/2014-10-31/">
  <DescribeDBInstancesResult>
    <Marker>page-2-marker</Marker>
    <DBInstances>
      <DBInstance>
        <DBInstanceIdentifier>pg-inst-1</DBInstanceIdentifier>
        <DBInstanceStatus>available</DBInstanceStatus>
        <Engine>postgres</Engine>
        <DBName>appdb</DBName>
        <IAMDatabaseAuthenticationEnabled>true</IAMDatabaseAuthenticationEnabled>
        <Endpoint>
          <Address>pg-inst-1.abc.us-east-1.rds.amazonaws.com</Address>
          <Port>5432</Port>
        </Endpoint>
        <TagList>
          <Tag><Key>env</Key><Value>prod</Value></Tag>
          <Tag><Key>datadoghq.com/dbm</Key><Value>true</Value></Tag>
          <Tag><Key>datadoghq.com/global_view_db</Key><Value>globaldb</Value></Tag>
        </TagList>
      </DBInstance>
    </DBInstances>
  </DescribeDBInstancesResult>
</DescribeDBInstancesResponse>`)
		case "page-2-marker":
			_, _ = io.WriteString(w, `<DescribeDBInstancesResponse xmlns="http://rds.amazonaws.com/doc/2014-10-31/">
  <DescribeDBInstancesResult>
    <DBInstances>
      <DBInstance>
        <DBInstanceIdentifier>mysql-inst-2</DBInstanceIdentifier>
        <DBClusterIdentifier>aurora-cluster-1</DBClusterIdentifier>
        <DBInstanceStatus>available</DBInstanceStatus>
        <Engine>aurora-mysql</Engine>
        <IAMDatabaseAuthenticationEnabled>false</IAMDatabaseAuthenticationEnabled>
        <Endpoint>
          <Address>mysql-inst-2.abc.us-east-1.rds.amazonaws.com</Address>
          <Port>3306</Port>
        </Endpoint>
        <TagList>
          <Tag><Key>env</Key><Value>prod</Value></Tag>
        </TagList>
      </DBInstance>
    </DBInstances>
  </DescribeDBInstancesResult>
</DescribeDBInstancesResponse>`)
		default:
			t.Fatalf("unexpected Marker %q", q.Get("Marker"))
		}
	}))
	defer ts.Close()

	svc, err := newRDSQueryClient(aws.Config{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(ts.URL),
		Credentials:  credentials.NewStaticCredentialsProvider("AKIAEXAMPLE", "SECRETEXAMPLE", ""),
		HTTPClient:   ts.Client(),
	})
	require.NoError(t, err)

	client := &Client{client: svc}
	instances, err := client.GetRdsInstancesFromTags(context.Background(), Config{
		Tags:            []string{"env:prod"},
		DbmTag:          defaultDbmTag,
		GlobalViewDbTag: defaultGlobalViewDbTag,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	assert.Equal(t, []Instance{
		{
			ID:           "pg-inst-1",
			Endpoint:     "pg-inst-1.abc.us-east-1.rds.amazonaws.com",
			Port:         5432,
			IamEnabled:   true,
			Engine:       "postgres",
			DbName:       "appdb",
			GlobalViewDb: "globaldb",
			DbmEnabled:   true,
		},
		{
			ID:         "mysql-inst-2",
			ClusterID:  "aurora-cluster-1",
			Endpoint:   "mysql-inst-2.abc.us-east-1.rds.amazonaws.com",
			Port:       3306,
			IamEnabled: false,
			Engine:     "aurora-mysql",
			DbName:     "mysql",
			DbmEnabled: false,
		},
	}, instances)
}

func TestRDSQueryClientDescribeDBClustersAndPagination(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		q, err := url.ParseQuery(string(body))
		require.NoError(t, err)
		assert.Equal(t, "DescribeDBClusters", q.Get("Action"))
		assert.Equal(t, "2014-10-31", q.Get("Version"))
		assert.Equal(t, "engine", q.Get("Filters.Filter.1.Name"))
		assert.Equal(t, "aurora-mysql", q.Get("Filters.Filter.1.Values.Value.1"))
		assert.Equal(t, "aurora-postgresql", q.Get("Filters.Filter.1.Values.Value.2"))

		w.Header().Set("Content-Type", "text/xml")
		switch q.Get("Marker") {
		case "":
			_, _ = io.WriteString(w, `<DescribeDBClustersResponse xmlns="http://rds.amazonaws.com/doc/2014-10-31/">
  <DescribeDBClustersResult>
    <Marker>cluster-page-2</Marker>
    <DBClusters>
      <DBCluster>
        <DBClusterIdentifier>aurora-cluster-1</DBClusterIdentifier>
        <TagList>
          <Tag><Key>team</Key><Value>dbm</Value></Tag>
        </TagList>
      </DBCluster>
    </DBClusters>
  </DescribeDBClustersResult>
</DescribeDBClustersResponse>`)
		case "cluster-page-2":
			_, _ = io.WriteString(w, `<DescribeDBClustersResponse xmlns="http://rds.amazonaws.com/doc/2014-10-31/">
  <DescribeDBClustersResult>
    <DBClusters>
      <DBCluster>
        <DBClusterIdentifier>aurora-cluster-2</DBClusterIdentifier>
        <TagList>
          <Tag><Key>team</Key><Value>dbm</Value></Tag>
        </TagList>
      </DBCluster>
      <DBCluster>
        <DBClusterIdentifier>aurora-cluster-skip</DBClusterIdentifier>
        <TagList>
          <Tag><Key>team</Key><Value>other</Value></Tag>
        </TagList>
      </DBCluster>
    </DBClusters>
  </DescribeDBClustersResult>
</DescribeDBClustersResponse>`)
		default:
			t.Fatalf("unexpected Marker %q", q.Get("Marker"))
		}
	}))
	defer ts.Close()

	svc, err := newRDSQueryClient(aws.Config{
		Region:       "eu-west-1",
		BaseEndpoint: aws.String(ts.URL),
		Credentials:  credentials.NewStaticCredentialsProvider("AKIAEXAMPLE", "SECRETEXAMPLE", ""),
		HTTPClient:   ts.Client(),
	})
	require.NoError(t, err)

	client := &Client{client: svc}
	clusters, err := client.GetAuroraClustersFromTags(context.Background(), []string{"team:dbm"})
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	require.Len(t, clusters, 2)
	assert.Equal(t, "aurora-cluster-1", aws.ToString(clusters[0].DBClusterIdentifier))
	assert.Equal(t, "aurora-cluster-2", aws.ToString(clusters[1].DBClusterIdentifier))
}

func TestRDSQueryClientErrorResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `<ErrorResponse xmlns="http://rds.amazonaws.com/doc/2014-10-31/">
  <Error>
    <Type>Sender</Type>
    <Code>AccessDenied</Code>
    <Message>User is not authorized to perform: rds:DescribeDBInstances</Message>
  </Error>
  <RequestId>rds-req-789</RequestId>
</ErrorResponse>`)
	}))
	defer ts.Close()

	svc, err := newRDSQueryClient(aws.Config{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(ts.URL),
		Credentials:  credentials.NewStaticCredentialsProvider("AKIAEXAMPLE", "SECRETEXAMPLE", ""),
		HTTPClient:   ts.Client(),
	})
	require.NoError(t, err)

	_, err = svc.DescribeDBInstances(context.Background(), &DescribeDBInstancesInput{})
	require.Error(t, err)

	var apiErr *awsquery.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, "AccessDenied", apiErr.Code)
	assert.Equal(t, "rds-req-789", apiErr.RequestID)
}

func TestRDSQueryClientAuroraEndpointsPagination(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		q, err := url.ParseQuery(string(body))
		require.NoError(t, err)
		assert.Equal(t, "DescribeDBInstances", q.Get("Action"))
		assert.Equal(t, "db-cluster-id", q.Get("Filters.Filter.1.Name"))
		assert.Equal(t, "my-aurora-cluster", q.Get("Filters.Filter.1.Values.Value.1"))

		w.Header().Set("Content-Type", "text/xml")
		switch q.Get("Marker") {
		case "":
			_, _ = io.WriteString(w, `<DescribeDBInstancesResponse>
  <DescribeDBInstancesResult>
    <Marker>aurora-page-2</Marker>
    <DBInstances>
      <DBInstance>
        <DBInstanceIdentifier>aurora-inst-1</DBInstanceIdentifier>
        <DBClusterIdentifier>my-aurora-cluster</DBClusterIdentifier>
        <DBInstanceStatus>available</DBInstanceStatus>
        <Engine>aurora-postgresql</Engine>
        <Endpoint><Address>inst-1.cluster.rds.amazonaws.com</Address><Port>5432</Port></Endpoint>
      </DBInstance>
    </DBInstances>
  </DescribeDBInstancesResult>
</DescribeDBInstancesResponse>`)
		case "aurora-page-2":
			_, _ = io.WriteString(w, `<DescribeDBInstancesResponse>
  <DescribeDBInstancesResult>
    <DBInstances>
      <DBInstance>
        <DBInstanceIdentifier>aurora-inst-2</DBInstanceIdentifier>
        <DBClusterIdentifier>my-aurora-cluster</DBClusterIdentifier>
        <DBInstanceStatus>available</DBInstanceStatus>
        <Engine>aurora-postgresql</Engine>
        <Endpoint><Address>inst-2.cluster.rds.amazonaws.com</Address><Port>5432</Port></Endpoint>
      </DBInstance>
    </DBInstances>
  </DescribeDBInstancesResult>
</DescribeDBInstancesResponse>`)
		default:
			t.Fatalf("unexpected Marker %q", q.Get("Marker"))
		}
	}))
	defer ts.Close()

	svc, err := newRDSQueryClient(aws.Config{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(ts.URL),
		Credentials:  credentials.NewStaticCredentialsProvider("AKIAEXAMPLE", "SECRETEXAMPLE", ""),
		HTTPClient:   ts.Client(),
	})
	require.NoError(t, err)

	client := &Client{client: svc}
	instances, err := client.GetAuroraClusterEndpoints(context.Background(), []DBCluster{
		{
			DBClusterIdentifier: aws.String("my-aurora-cluster"),
			TagList:             []Tag{{Key: aws.String("datadoghq.com/dbm"), Value: aws.String("true")}},
		},
	}, Config{DbmTag: defaultDbmTag})
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	require.Len(t, instances, 2)
	assert.Equal(t, "aurora-inst-1", instances[0].ID)
	assert.Equal(t, "aurora-inst-2", instances[1].ID)
	assert.True(t, instances[0].DbmEnabled)
	assert.True(t, instances[1].DbmEnabled)
}
