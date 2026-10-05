rule MythStealer
{
	meta:
		author = "Still"
		component_name = "MythStealer"
		date = "2025-06-13"
		description = "attempts to match the strings/instructions found in MythStealer"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_reference_md5 = "3ed2ea6c74122b78b8ef83a0dcf6eb4c"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "548efbd6-2db9-4420-a1ea-3e5210bd3aa5"
	strings:
		$module_parent_1 = "myth\\" ascii
		$module_parent_2 = "\\steal\\" ascii
		$module_parent_3 = "\\clipper\\" ascii
		$module_child_1 = "\\browser.rs" ascii
		$module_child_2 = "\\discord.rs" ascii
		$module_child_3 = "\\checks.rs" ascii
		$module_child_4 = "\\v20_decrypt.rs" ascii
		$str_1 = "orospu evladi.... " ascii
		$str_2 = "oh no: " ascii
		$str_3 = "OpenProcess failed. Likely missing SeDebugPrivilege.\n" ascii
		$str_4 = "error while decrypt v20" ascii
		$str_5 = "Error while sql connection" ascii
	condition:
		(
			2 of ($module_parent_*) and
			2 of ($module_child_*)
		) or
		3 of ($str_*)
}
