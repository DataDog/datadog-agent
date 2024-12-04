$ErrorActionPreference = 'Stop'

$maven_sha512 = '8BEAC8D11EF208F1E2A8DF0682B9448A9A363D2AD13CA74AF43705549E72E74C9378823BF689287801CBBFC2F6EA9596201D19CCACFDFB682EE8A2FF4C4418BA'

if ("$env:WITH_JMX" -ne "false") {
    cd \fips-build
    Invoke-WebRequest -Outfile maven.zip https://dlcdn.apache.org/maven/maven-3/3.9.9/binaries/apache-maven-3.9.9-bin.zip
    if ((Get-FileHash -Algorithm SHA512 maven.zip).Hash -eq $maven_sha512) {
        Write-Host "Maven checksum match"
    } else {
        Write-Error "Checksum mismatch"
    }
    Expand-Archive -Force -Path maven.zip -DestinationPath .
    .\apache-maven-3.9.9\bin\mvn -D maven.repo.local=maven-repo dependency:copy-dependencies
    New-Item -Force -ItemType directory -Path 'C:/Program Files/Datadog/BouncyCastle FIPS/'
    Move-Item -Force -Path @("target/dependency/*.jar", "java.security", "bc-fips.policy") 'C:/Program Files/Datadog/BouncyCastle FIPS/'
    \java\bin\java --module-path 'C:\Program Files\Datadog\BouncyCastle FIPS' org.bouncycastle.util.DumpInfo
    if (!$?) {
        Write-Error ("BouncyCastle self check failed with exit code: {0}" -f $LASTEXITCODE)
    }
}
cd \
Remove-Item -Force -Recurse \fips-build
