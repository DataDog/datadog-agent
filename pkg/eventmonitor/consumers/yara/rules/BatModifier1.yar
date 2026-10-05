rule BatModifier1
{
    meta:
        author = "Madhav"
        description = "This is a bat file which is setup a game. 49509"
        date = "2025-05-10"
	yarahub_reference_md5 = "79a546f11d5ed65736735ba86cb95213"
	yarahub_uuid = "fb799bc3-fe63-40cd-804c-28a821d99c5b"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
    
    strings:
        $mal1 = "PowerShell -Command \"Start-Process '%~f0' -Verb runAs\""
        $mal2 = "net session"
        $mal3 = "powershell -Command \"Invoke-WebRequest -Uri"
        $mal4 = "%SystemRoot%\\System32\\drivers\\etc\\hosts"
        $mal5 = "netsh advfirewall firewall add rule"
        $mal6 = "dir=out action=block remoteip="
	$mal7 = "%SystemRoot%\\System32\\curl.exe"
	$mal8 = "shell \"su -c 'id'\" | find \"uid=0(root)\""
	$mal9 = "tinyurl.com"
	$mal10 = "TaskKill /F /IM"
	$mal11 = "reg delete"
	$mal12 = "rd /s /q"
	$mal13 = "rd /q /s"
	$mal14 = "copy /y"
	$mal15 = "del /f"
	$mal16 = "del /s"
	$mal17 = "del /q"
    
    condition:
        all of ($mal1, $mal2, $mal3, $mal4, $mal5, $mal6, $mal7, $mal8) and
    	2 of ($mal9, $mal10, $mal11, $mal12, $mal13, $mal14, $mal15, $mal16, $mal17)
}
