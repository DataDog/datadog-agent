// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

/*
Package checkmetadata implements the Check Metadata provider.

This sends info such as versions of applications monitored, configuration options used, etc.

The collector keeps a cache of check metadata `checkMetadataCache` and exports the function
`SetCheckMetadata` so that entries can be added from other packages. This metadata provider
is unlike most others in that it doesn't actually collect any info, but rather it simply
sends whatever it finds stored in the cache. The cache is cleared at every collection.
*/
package checkmetadata
