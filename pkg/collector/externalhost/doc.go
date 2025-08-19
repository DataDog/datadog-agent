// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

/*
Package externalhost implements the External Host Tags metadata provider.

In older versions of the Agent, it was the general metadata collector to invoke
a special method on the check instance to collect a list of tags attached to a
specific hostname, add this tuple to the metadata payload and send upstream. At
this moment the approach has changed from the Agent "pulling" from the check, to
the check "pushing" to the Agent. See the RFC at docs/proposal/metadata/external-host-tags.md
for more details.

The collector keeps a cache of hostnames mapped to a list of tags called `externalHostCache`
and exports the function `AddExternalTags` so that entries can be added from
other packages. This metadata provider is different from the others because it
doesn't actually collect any info, it only sends whatever it finds stored in the
cache. The cache is cleared at every collection.
*/
package externalhost
