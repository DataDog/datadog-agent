rule win_Eternity
{
	meta:
		author = "0xToxin"
		description = "Eternity function routines"
		date = "2022-12-10"
		yarahub_reference_md5 = "cb1b7d3a9bd4f3742c3b8c4c21c808b8"
		yarahub_license = "CC BY 4.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		malpedia_family = "win.eternity_stealer"
		yarahub_uuid = "8af629d9-206a-4d75-acd2-f6b21ae9b4ac"
	strings:
		$string_xor_routine = {
			5D
			?? ?? 00 00 0A
			61
			D1
		}
		
		$switch_case = {
			FE 0C 00 00
			FE 0C 01 00
			93
			?? ?? 00 00 0A
		}
	condition:
		uint16(0) == 0x5a4d and $string_xor_routine and #switch_case >= 3
	}