rule AppLaunch
{
	meta:
		author = "iam-py-test"
		description = "Detect files referencing .Net AppLaunch.exe"
		example_file = "ba85b8a6507b9f4272229af0606356bab42af42f5ee2633f23c5e149c3fb9ca4"
		new_example_file = "cda99e504a122208862739087cf16b4838e9f051acfcbeb9ec794923b414c018"
		in_the_wild = true
		// yarahub data
		date = "2022-11-17"
		yarahub_uuid = "613f8ac7-a5f3-4167-bbcd-4dbfd4c8ba67"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_reference_md5 = "7dbfe0186e52ef2da13079f6d5b800d7"
	strings:
		$filelocation = "C:\\Windows\\Microsoft.NET\\Framewor"
		$applaunch = "\\AppLaunch.exe" nocase
	condition:
		$filelocation and $applaunch
}