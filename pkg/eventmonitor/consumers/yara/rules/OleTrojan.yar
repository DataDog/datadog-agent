rule OleTrojan
{
    meta:
        author = "Madhav"
        description = "This is a ole file which is accessing some url. 49496"
        date = "2025-05-09"
	yarahub_reference_md5 = "7a8c0555498fa12e5ae846f7e5dd0dbf"
	yarahub_uuid = "9e2f0ead-9358-4479-8831-26e4d51812e2"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
    
    strings:
        $mal1 = "LinkFormat.SourceFullName"
        $mal2 = "Sub Document_Open"
        $mal3 = "ADODB.Stream"
        $mal4 = "InternetExplorer.Application"
        $mal5 = "SetForegroundWindow"
        $mal6 = "Content-Type: application/x-www-form-urlencoded"
    
    condition:
        $mal1 and $mal2 and $mal3 and $mal4 and $mal5 and $mal6
}