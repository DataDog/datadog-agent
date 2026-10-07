rule KRUEGER {
	meta :
		description = "Identifies a Krueger binary"
		author = "Jonathan Beierle"
		reference = "https://github.com/logangoins/Krueger"
		date = "2024-12-17"
		yarahub_reference_md5 = "ae22487ff5fb08c07f515dfccde6d11e"
		yarahub_uuid = "7ed4e8b2-7966-4fca-a6ed-8d514724aa63"
		yarahub_license = "CC BY 4.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
	strings:
		$policydst1 = "\\C$\\Windows\\System32\\CodeIntegrity\\SiPolicy.p7b"
		$policydst2 = "ADMIN$\\System32\\CodeIntegrity\\SiPolicy.p7b"

		/* Strings and bytes used to identify an embedded WDAC policy */
		$wdac1 = { 0E 37 44 A2 C9 44 06 4C B5 51 F6 01 6E 56 30 76 }  /* Bytes used for several section headers in WDAC policies */
		$wdac2 = "_?r"
		$wdac3 = "PTbS^}"
		$wdac4 = "TJ-"

		$s1 = "Krueger.exe"
		$s2 = "Krueger.SiPolicy.p7b"
	condition:
		(  /* Test for embedded WDAC policy */
			all of ($wdac*) and
			#wdac1 >= 3
		) and 
		any of ($s*) or
		any of ($policydst*)
}