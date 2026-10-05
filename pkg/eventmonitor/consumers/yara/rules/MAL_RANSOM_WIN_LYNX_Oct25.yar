rule MAL_RANSOM_WIN_LYNX_Oct25
{
	meta:
		author = "VolkerLieber"
		description = "Detects windows version of LYNX ransomware"
		date = "2025-10-08"
		hash = "044e2db9c8aafd35448c5f93675bb712cf16ebc3c6866f6a990481268db26257"
		yarahub_reference_md5 = "34876c303ccc697763449d5598a8c5d5"
		yarahub_uuid = "1401af50-710d-40a1-9179-17a2e3ec668e"
		yarahub_license = "CC BY 4.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
	strings:
		$str0 = "Starting full encryption in " wide
		$str1 = "Couldn\\'t delete shadow copies from %c:" wide
		$str2 = "Enable silent encryption (no extension and notes will be added)" wide

		// encryption mode parsing
		$op1 = { 0F B7 09 83 E9 65 74 ?? 83 E9 01 74 ?? 83 E9 07 74 ?? 83 F9 06 74 ?? }
		// generate file encryption keys
		$op2 = { 48 89 5F 38 FF 15 ?? ?? ?? ?? 33 D2 48 8B C8 44 8D 42 20 FF 15 ?? ?? ?? ?? 48 89 47 40 FF 15 ?? ?? ?? ?? 33 D2 48 8B C8 44 8D 42 40 FF 15 ?? ?? ?? ?? 48 89 47 48 FF 15 ?? ?? ?? ?? 33 D2 48 8B C8 44 8D 42 40 FF 15 ?? ?? ?? ?? 48 89 47 50 48 39 77 30 }
		// set shadow copy size to zero
		$op3 = { 41 B9 18 00 00 00 BA 28 C0 53 00 48 89 5C 24 20 48 8B CE FF 15 ?? ?? ?? ?? 85 C0 74 ?? }
	condition:
		// PE magic number
		uint16(0) == 0x5A4D and
		(
			2 of ($str*) or
			2 of ($op*) or
			(
				1 of ($str*) and
				1 of ($op*)
			)
		)
}
