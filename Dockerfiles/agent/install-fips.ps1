$ErrorActionPreference = 'Stop'

# The BouncyCastle FIPS provider only serves JMX FIPS images
if ("$env:WITH_JMX" -ne "false" -and "$env:WITH_FIPS" -eq "true") {
    \java\bin\java org.bouncycastle.util.DumpInfo
    if (!$?) {
        Write-Error ("BouncyCastle self check failed with exit code: {0}" -f $LASTEXITCODE)
    }
} else {
    Remove-Item -Force -Recurse 'C:/Program Files/Datadog/BouncyCastle FIPS'
}

if ("$env:WITH_FIPS" -ne "true") {
    # If FIPS is not enabled, skip the FIPS setup
    exit 0
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
