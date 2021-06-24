(New-Object System.Net.WebClient).DownloadFile("https://curl.haxx.se/ca/cacert.pem", "c:\cacert.pem")
#if ((Get-FileHash .\cacert.pem).Hash -ne \"$ENV:CACERTS_HASH\") { Write-Host \"Wrong hashsum for cacert.pem: got '$((Get-FileHash .\cacert.pem).Hash)', expected '$ENV:CACERTS_HASH'.\"; exit 1 }
setx SSL_CERT_FILE "C:\cacert.pem"
