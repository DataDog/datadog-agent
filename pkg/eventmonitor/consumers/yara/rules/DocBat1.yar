rule DocBat1
{
    meta:
        author = "Madhav"
        description = "This is a bat file which is a info stealer which take SS of the desktop and sends it to the attackers discord webhook using curl"
        date = "2025-05-25"
	yarahub_reference_md5 = "122806cf4b66b138befe236744fd9a0f"
	yarahub_uuid = "375e4c9a-0628-4464-a75f-1a3935c1d3e4"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
    
    strings:
        $mal1 = "echo. > %appdata%\\ "
        $mal2 = "start /MIN"
        $mal3 = "del /F \"%appdata%\\"
        $mal4 = "C:\\Users\\%username%\\AppData\\Roaming\\Microsoft\\Windows\\Start Menu\\Programs\\Startup"
        $mal5 = "powershell -command"
        $mal6 = "Get-Clipboard"
	$mal7 = "Add-Type -AssemblyName System.Windows.Forms"
	$mal11 = "SendWait('{PRTSC}')" 
	$mal12 = ".Clipboard]::GetImage()" 
	$mal13= ".Save("
	$mal8 = "nslookup myip.opendns.com resolver1.opendns.com"
	$mal9 = "\\Google\\Chrome\\User Data\\Default\\Login Data"
	$mal10 = "curl -X POST"

    
    condition:
        any of ($mal1, $mal2, $mal3, $mal8, $mal4, $mal9) and (($mal5 and $mal6) or ($mal5 and $mal7 and $mal11 and $mal12 and $mal13)) and $mal10
}