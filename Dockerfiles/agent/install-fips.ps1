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

$maven_sha512 = '2e24dbea0407489d45b4d8214afff96fb57b54a5ef2bb6878f65fbce9b4141685b878ec5c53e9d07d4b1bf166bb5c4f80d540a13013b133a250ec9d85effa37c'

if ("$env:WITH_JMX" -ne "false") {
    cd \fips-build
    Invoke-WebRequest -Outfile maven.zip https://dlcdn.apache.org/maven/maven-3/3.9.10/binaries/apache-maven-3.9.10-bin.zip
    if ((Get-FileHash -Algorithm SHA512 maven.zip).Hash -eq $maven_sha512) {
        Write-Host "Maven checksum match"
    } else {
        Write-Error "Checksum mismatch"
    }
    Expand-Archive -Force -Path maven.zip -DestinationPath .
    .\apache-maven-3.9.10\bin\mvn -D maven.repo.local=maven-repo dependency:copy-dependencies
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
