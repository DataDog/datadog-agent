
Get-ChildItem 'entrypoint-ps1' | ForEach-Object {
	& $_.FullName
	if (-Not $?) {
		exit 1
	}
}

$agent = $args[0]
$agentArgs = $args | Select-Object -Skip 1
return & $agent $agentArgs
