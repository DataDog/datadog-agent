"""A module initialising the third party dependencies OpenSSL"""

load("@rules_perl//perl:deps.bzl", "perl_register_toolchains", "perl_rules_dependencies")

def openssl_setup():
    perl_rules_dependencies()
    perl_register_toolchains()
