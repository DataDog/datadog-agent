rule AteraAgent_RemoteAdmin_April_2024 {
    meta:
        author = "NDA0"
        date = "2024-04-16"
        description = "Detects AteraAgent Remote Admin Tool"
        yarahub_uuid = "0a9aaf26-0d26-41a2-853b-08cdde61306d"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "4786b508296d522bde9b35893599f677"
    strings:
	$AteraAgent1 = "AteraAgent.exe" ascii
	$config = "AteraAgent.exe.config" ascii
	$AteraAgentInstaller = "This installer database contains the logic and data required to install AteraAgent" ascii
	$AteraAgent2 = "AteraAgent" ascii
	$AteraNetworks = "Atera networks" ascii
	$TARGETDIR = "TARGETDIRAteraAgent.exe" ascii
	$INSTALLFOLDER = "INSTALLFOLDERAteraAgent.exe.config" ascii
	$Signature = "Atera Networks Ltd" ascii
    condition:
        $AteraAgent1 and $config and $AteraAgentInstaller and $AteraAgent2 and any of them
}