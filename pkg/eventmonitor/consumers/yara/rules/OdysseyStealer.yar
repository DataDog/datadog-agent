rule OdysseyStealer
{
	meta:
		author = "Still"
		component_name = "OdysseyStealer"
		date = "2025-07-12"
		description = "attempts to match the strings found in OdysseyStealer"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_reference_md5 = "e807e2bf37ff5a8b1aa7f1d239564647"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "747a2aea-7ce7-4855-965e-dacefda2be4e"
	strings:
		$str_1 = "/tmp/lovemrtrump/"
		$str_2  ="\\\"/.pwd\\\""
		$str_3 = "\\\"<h1>Notes Count: \\\""
		$str_4 = "\\\"Required Application Helper. Please enter device password to continue.\\\""
		$str_5 = "\"buildid: $BUILDID$"
		$str_6 = "\\\"finder/saf1\\\""
		$str_7 = "/tmp/socks\\\""
		$str_8 = " to do shell script \\\"dscl . authonly \\\" & quoted form of"
		$str_9 = "rm /tmp/out.zip\\\""
	condition:
		3 of them
}
