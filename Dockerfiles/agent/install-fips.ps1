if ("$env:WITH_JMX" -ne "false") {
    cd \fips-build
    Invoke-WebRequest -Outfile maven.zip https://dlcdn.apache.org/maven/maven-3/3.9.9/binaries/apache-maven-3.9.9-bin.zip
    (Get-FileHash -Algorithm SHA512 maven.zip).Hash -eq "8BEAC8D11EF208F1E2A8DF0682B9448A9A363D2AD13CA74AF43705549E72E74C9378823BF689287801CBBFC2F6EA9596201D19CCACFDFB682EE8A2FF4C4418BA"
    Expand-Archive -Path maven.zip -DestinationPath C:/
    Remove-Item maven.zip
    apache-maven-3.9.9\bin\mvn -Dmaven.repo.local=maven-repo dependency:copy-dependencies
    New-Item -ItemType directory -Path 'C:/Program Files/Datadog/BouncyCastle FIPS/'
    Move-Item "target/dependency/*.jar" java.security bc-fips.policy 'C:/Program Files/Datadog/BouncyCastle FIPS/'
}
Remove-Item -Recurse \fips-build
