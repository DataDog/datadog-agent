"""Bazel constants for common configuration values."""

# macOS Code Signing Identity
# Used for signing native libraries and binaries on macOS
# This should match the identity used in omnibus/config/projects/agent.rb
apple_signing_identity = "Developer ID Application: Datadog, Inc. (JKFCB4CN7C)"
