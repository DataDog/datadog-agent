# Embedding the Remote Configuration service

The existing RC service can run in another process without importing Agent application or Collector components. `service.WithAPIClient` supplies its backend API implementation. `api.WithHTTPClient` supplies a caller-owned HTTP client when using the standard backend protocol.

The caller owns HTTP configuration, request cancellation, redirects and connection cleanup. The service retains signature verification, caching, client tracking and polling. These APIs do not register an RC listener or change the default Agent service.
