rule BatModifier3
{
    meta:
        author = "Madhav"
        description = "This is a bat file which is setup a game. 49509"
        date = "2025-05-10"
	yarahub_reference_md5 = "79a546f11d5ed65736735ba86cb95213"
	yarahub_uuid = "40a63190-bedb-445f-ad61-bf142ed03ca3"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
    
    strings:
        $mal1 = "PowerShell -Command \"Start-Process '%~f0' -Verb runAs\""

        $mal3 = "Invoke-WebRequest -Uri"
        $mal4 = "%SystemRoot%\\System32\\drivers\\etc\\hosts"
        $mal5 = "netsh advfirewall firewall add rule"

	$mal7 = "%SystemRoot%\\System32\\curl.exe"
	$mal8 = "shell \"su -c 'id'\""
	$mal15 = "uid=0(root)"
	$mal10 = "TaskKill /F /IM"
	$mal11 = "reg delete"
	$mal12 = "rd /"
	$mal13 = "copy /"
	$mal14 = "del /"
    
    condition:
        all of ($mal1, $mal3, $mal4, $mal5, $mal7) and 2 of ($mal8, $mal15, $mal10, $mal11, $mal12, $mal13, $mal14)
}