$ErrorActionPreference = 'Stop'

# Removes temporary files for FIPS setup
function Remove-TempFiles {
    Remove-Item -Force -Recurse \fips-build
}

if ("$env:WITH_FIPS" -ne "true") {
    # If FIPS is not enabled, skip the FIPS setup
    Remove-TempFiles
    exit 0
}

$maven_sha512 = '03e2d65d4483a3396980629f260e25cac0d8b6f7f2791e4dc20bc83f9514db8d0f05b0479e699a5f34679250c49c8e52e961262ded468a20de0be254d8207076'
$maven_version = '3.9.11'

if ("$env:WITH_JMX" -ne "false") {
    cd \fips-build
    Invoke-WebRequest -Outfile maven.zip https://archive.apache.org/dist/maven/maven-3/${maven_version}/binaries/apache-maven-${maven_version}-bin.zip
    if ((Get-FileHash -Algorithm SHA512 maven.zip).Hash -eq $maven_sha512) {
        Write-Host "Maven checksum match"
    } else {
        Write-Error "Checksum mismatch"
    }
    Expand-Archive -Force -Path maven.zip -DestinationPath .
    & ".\apache-maven-${maven_version}\bin\mvn" -D maven.repo.local=maven-repo dependency:copy-dependencies
    New-Item -Force -ItemType directory -Path 'C:/Program Files/Datadog/BouncyCastle FIPS/'
    Move-Item -Force -Path @("target/dependency/*.jar", "java.security", "bc-fips.policy") 'C:/Program Files/Datadog/BouncyCastle FIPS/'
    \java\bin\java --module-path 'C:\Program Files\Datadog\BouncyCastle FIPS' org.bouncycastle.util.DumpInfo
    if (!$?) {
        Write-Error ("BouncyCastle self check failed with exit code: {0}" -f $LASTEXITCODE)
    }
    cd \
}

# Configure Python's OpenSSL FIPS module
# The OpenSSL security policy states:
# "The Module shall have the self-tests run, and the Module config file output generated on each
#  platform where it is intended to be used. The Module config file output data shall not be copied from
#  one machine to another."
# https://github.com/openssl/openssl/blob/master/README-FIPS.md
# We provide the -self_test_onload option to ensure that the install-status and install-mac options
# are NOT written to fipsmodule.cnf. This allows us to create the config during the image build,
# and means the self tests will be run on every container start.
# https://docs.openssl.org/master/man5/fips_config
# Discussion about putting the commands in image vs entrypoint:
# https://github.com/openssl/openssl/discussions/23920
$embeddedPath = "C:\Program Files\Datadog\Datadog Agent\embedded3"
$fipsProviderPath = "$embeddedPath\lib\ossl-modules\fips.dll"
$fipsConfPath = "$embeddedPath\ssl\fipsmodule.cnf"
& "$embeddedPath\bin\openssl.exe" fipsinstall -module "$fipsProviderPath" -out "$fipsConfPath" -self_test_onload
$err = $LASTEXITCODE
if ($err -ne 0) {
    Write-Error ("openssl fipsinstall exited with code: {0}" -f $err)
    exit $err
}
# Run again with -verify option
& "$embeddedPath\bin\openssl.exe" fipsinstall -module "$fipsProviderPath" -in "$fipsConfPath" -verify
$err = $LASTEXITCODE
if ($err -ne 0) {
    Write-Error ("openssl fipsinstall verification of FIPS compliance failed, exited with code: {0}" -f $err)
    exit $err
}
# We don't need to modify the .include directive in openssl.cnf here because the container
# always uses the default installation path.
$opensslConfPath = "$embeddedPath\ssl\openssl.cnf"
$opensslConfTemplate = "$embeddedPath\ssl\openssl.cnf.tmp"
Copy-Item "$opensslConfTemplate" "$opensslConfPath"

# Configure Windows FIPS mode
# This system-wide setting is used by Windows as well as the Microsoft Go fork used by the Agent
# https://github.com/microsoft/go/blob/microsoft/main/eng/doc/fips/README.md#windows-fips-mode-cng
Set-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Control\Lsa\FipsAlgorithmPolicy" -Name "Enabled" -Value 1 -Type DWORD

Remove-TempFiles
