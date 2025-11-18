"""A centralized module initializing repositories required for third party examples of rules_foreign_cc which require loading from repositories which themselves were loaded in repositories.bzl."""

load("//openssl:openssl_setup.bzl", "openssl_setup")

def setup():
    openssl_setup()
