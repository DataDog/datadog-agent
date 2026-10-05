rule DanaBot_12_2023 {

	meta:
		author = "RussianPanda"
		decription = "Detects the new version of DanaBot"
		date = "2023-12-01"
		yarahub_author_twitter = "@AnFam17"
		yarahub_author_email = "cyberninja956@gmail.com"
		yarahub_reference_md5 = "d3fa8e6816f5a99fc9218192f02e7611"
		yarahub_uuid = "5329ec2a-43a9-410c-abdc-a355b5a2ae2b"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		malpedia_family = "win.danabot"

	strings:
		$s1 = {55 8b ec 8a 45 08 34 4a 5d c2 04 00}
		$s2 = {4D 00 6F 00 7A 00 69 00 6C 00 6C 00 61 00 5C 00 53 00 65 00 61 00 4D 00 6F 00 6E 00 6B 00 65 00 79}
		$s3 = {4D 00 6F 00 7A 00 69 00 6C 00 6C 00 61 00 20 00 54 00 68 00 75 00 6E 00 64 00 65 00 72 00 62 00 69 00 72 00 64 00}
		$s4 = {53 00 6F 00 66 00 74 00 77 00 61 00 72 00 65 00 5C 00 4F 00 52 00 4C 00 5C 00 57 00 69 00 6E 00 56 00 4E 00 43 00 33 00 5C 00 50 00 61 00 73 00 73 00 77 00 6F 00 72 00 64}
		$s5 = {53 00 6F 00 66 00 74 00 77 00 61 00 72 00 65 00 5C 00 45 00 78 00 63 00 69 00 74 00 65 00 5C 00 50 00 72 00 69 00 76 00 61 00 74 00 65 00 4D 00 65 00 73 00 73 00 65 00 6E 00 67 00 65 00 72 00 5C 00 50 00 61 00 73 00 73 00 77 00 6F 00 72 00 64}
		$a = {44 45 4C 50 48 49 43 4C 41 53 53}
	condition:
		3 of ($s*) and $a
}

