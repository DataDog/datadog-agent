
Get-ChildItem 'entrypoint-ps1' | ForEach-Object {
	& $_.FullName
	if (-Not $?) {
		exit 1
	}
}

return & "C:/Program Files/Datadog/Datadog Agent/bin/agent.exe" $args
