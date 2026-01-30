OWNER: tony.aiuto@datadoghq.com
PROJECT: https://datadoghq.atlassian.net/browse/ABLD-158

Temporary clone of nghttp fetch-oscp-response
Copied from output of previous build with configure make.

This file is included with nghttp2-1.58, but was dropped from the
distribution sometime since then, and is longer part of 1.68.

We copy it here to allow us to move to a newer version of nghttp.

It is not clear that this is needed in the agent product. We may have been
including it accidentally, or as something needed when there used to be Python 2
support. Ideally we will eventually delete this.
