#!/bin/sh

# Generate certificates for E2E JMXFetch mTLS tests using Agent docker images.

set -e

# An image containing stock JVM
JAVA_IMAGE="${JAVA_IAMGE:-datadog/agent:latest-jmx}"
# An image containing a JVM configured to use BouncyCastle FIPS module
FIPS_IMAGE="${FIPS_IMAGE:-datadog/agent:latest-fips-jmx}"
# Really long time to avoid breaking tests. These certificates are not
# to be used for anything important.
VALIDITY=$((365*100))

BASEDIR=$(pwd)

docker() {
    echo -n "\n\033[1m"
    echo -n "docker $@"
    echo "\033[0m"
    command docker "$@"
}

dkeytool() {
    docker run --rm -v "$BASEDIR:/ssl" --workdir /ssl --entrypoint keytool "$@"
}

generate() {
    dkeytool "$image" -keystore "$prefix/$name-keystore" $pass_opts -genkey -alias "$name" -dname "CN=$name" $cert_opts
    dkeytool "$image" -keystore "$prefix/$name-keystore" $pass_opts -export -alias "$name" -rfc -file "$prefix/$name-cert.pem"
}

trust() {
    case $name in
        jmxfetch) other=java-app;;
        java-app) other=jmxfetch;;
        *) echo "Invalid side name: $name"; exit 1;;
    esac

    dkeytool "$image" -keystore "$prefix/$name-truststore" $pass_opts -import -alias "$other-pkcs12" -noprompt -file "pkcs12/$other-cert.pem"
    dkeytool "$image" -keystore "$prefix/$name-truststore" $pass_opts -import -alias "$other-bcfks"  -noprompt -file "bcfks/$other-cert.pem"
}

# This is the default Java password keystore password, there is nothing secret about it.
pass_opts="-storepass changeit"
cert_opts="-validity $VALIDITY -keyalg ec"

mkdir -p pkcs12 bcfks

for stage in generate trust; do
    for name in java-app jmxfetch; do
        while read prefix image; do
            $stage
        done <<EOF
pkcs12 $JAVA_IMAGE
bcfks  $FIPS_IMAGE
EOF
    done
done

