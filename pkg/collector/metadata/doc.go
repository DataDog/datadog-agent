/*
Package metadata implements specific Metadata Collectors for the Agent. Such
collectors might have dependencies (like Python) that we don't want in the
general purpose `github.com/DataDog/datadog-agent/pkg/metadata` package,
that can be imported by different softwares like Dogstatsd.
*/
package metadata
