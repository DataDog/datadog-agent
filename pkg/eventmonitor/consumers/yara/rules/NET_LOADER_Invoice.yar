rule NET_LOADER_Invoice : FILE
{
	meta:
		description = ".NET loader with payload in lsmsix.jpg"
		author = "daschr"
		date = "2024-05-28"
		modified = "2024-05-28"
		tags = "FILE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_license = "CC BY 4.0"
		yarahub_uuid = "31f2000e-784c-4fff-ac0d-421e1387d4da"
		yarahub_reference_md5 = "1e85f3371c212d315deeebf30866f282"

	strings:
		$s1 = "resources/lsmsix.jpg" ascii wide 
		$s2 = "y8F4Zm" ascii
		$s3 = "Zy0a2M" ascii
		$s4 = "Bx9k1F" ascii
		$s5 = {
			0a 0d 0d 2e 65 5b 6f 6d 20 53 4f 7b 20 6e 69 20 6e
			4a 72 20 65 62 20 4b 6f 6e 6e 61 63 1f 6d 61 72
			67 6f 4d 70 20 73 69 68 6b 21 cd 4c 01 b8 1e cd
			09 b4 00 0e 85 1f 0e 00 00 00 bf 00 00 00 00 00
			3f 00 00 00 00 00 3f 00 00 00 00 00 3f 00 00 00
			00 00 3f 00 00 00 00 00 3f 00 00 00 00 00 7f 00
			00 00 00 00 3f 00 b8 00 00 ff c0 00 00 00 04 00
			3f 00 03 00 90 5a 72
		}
		
	condition:
		filesize < 2MB and uint32(0) == 0x00905a4d and ( 3 of ($s*) ) 
}


