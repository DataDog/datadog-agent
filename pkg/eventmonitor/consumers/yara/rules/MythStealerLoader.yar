rule MythStealerLoader {
	meta:
		author = "Still"
		component_name = "MythStealer"
		date = "2025-06-13"
		description = "attempts to match the strings/instructions found in MythStealer loader; this is a very loose rule and may match fp"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_reference_md5 = "7a98967deb6b10311ab6d12e8bd5a144"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "a4702e4d-51fc-42f3-9813-fe26de5b9452"
	strings:
		$str_1 = "loader\\src\\main.rs" ascii
		$str_2 = "PeLoaderErr" ascii
		$str_3 = "memexec" ascii
	condition:
		all of them
}
