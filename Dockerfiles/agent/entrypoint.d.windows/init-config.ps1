Get-ChildItem 'entrypoint-ps1' | ForEach-Object { & $_.FullName if (-Not $?) { exit 1 } }
