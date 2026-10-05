rule CRU_Xerph_Stealer_v0 {
	meta:
		author = "ConnectWise CRU"
		researcher = "Blake Eakin"
		description = "Detects window hiding and mutex creation routine of Xerph Stealer"
		date = "2025-04-09"
		os = "Windows"
		yarahub_uuid = "432c06c6-5ac8-4f65-9dd5-78e78d53be69"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_reference_md5 = "71d4b3b2de5ae48cc98a5de6a231948a"
	strings:
		$init = {8D 45 F4 64 A3 00 00 00 00 FF 15 ?? ?? ?? ?? 85 C0 74 10 6A 00 50 FF 15 ?? ?? ?? ?? C7 45 FC FF FF FF FF 68 ?? ?? ?? ?? 6A 00 6A 00 FF 15 ?? ?? ?? ?? FF 15 ?? ?? ?? ?? 3D B7 00 00 00 75 ??}
	condition:
		all of them
}