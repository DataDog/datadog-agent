rule Win_DarkGate
{
	meta:
		author = "0xToxin"
		description = "DarkGate Strings Decryption Routine"
		date = "2023-08-01"
		yarahub_reference_md5 = "152ea1d672c7955f3da965dc320dc170"
		yarahub_license = "CC BY 4.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "9e190198-c38c-405b-a810-0a4c1b5b6db0"
	strings:
		$chunk_1 = {
			8B 55 ??
			8A 4D ??
			80 E1 3F
			C1 E1 02
			8A 5D ??
			80 E3 30
			81 E3 FF 00 00 00
			C1 EB 04
			02 CB
			88 4C 10 ??
			FF 45 ??
			80 7D ?? 40
			74 ??
			8B 45 ??
			E8 ?? ?? ?? ??
			8B 55 ??
			8A 4D ??
			80 E1 0F
			C1 E1 04
			8A 5D ??
			80 E3 3C
			81 E3 FF 00 00 00
			C1 EB 02
			02 CB
			88 4C 10 ??
			FF 45 ??
			80 7D ?? 40
			74 ??
			8B 45 ??
			E8 ?? ?? ?? ??
			8B 55 ??
			8A 4D ??
			80 E1 03
			C1 E1 06
			8A 5D ??
			80 E3 3F
			02 CB
			88 4C 10 ??
			FF 45 ??
		}
	
	condition:
		any of them
}
