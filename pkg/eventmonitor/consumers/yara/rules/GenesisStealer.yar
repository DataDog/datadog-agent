rule GenesisStealer
{
	meta:
		author = "Still"
		component_name = "GenesisStealer"
		date = "2026-01-25"
		description = "attempts to match strings found in GenesisStealer and its artifacts"
		yarahub_uuid = "88298ad9-30d9-4c06-8b38-6bd78559b5cd"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_reference_md5 = "6c03c7ba770d3e4a27f8b571ed7dca6f"
	strings:
		$js_str_1 = "t.me/GenesisSociety" ascii
		$js_str_2 = "Opera Data Extracted From Victim Browsers" ascii
		$js_str_3 = "Error extracting autofills from" ascii
		$js_str_4 = "\\u26A0\\uFE0F Le " ascii
		$js_str_5 = "s de la cr\\xE9a" ascii
		$js_str_6 = "GENESIS-MONTHLY-" ascii
		$js_str_7 = "GENESIS-WEEKLY-" ascii
		$js_str_8 = "uploads.kalygenesis.xyz" ascii
		$js_str_9 = "Erreur lor" ascii
		$js_str_10 = "bmV3IEZ1bmN0aW9uKCJyZXF1aXJlIiwgZGVjcnlwdGVkKShyZXF1aXJlKTs" ascii
		$py_str_1 = "This script must be run as an administrator." ascii
		$py_str_2 = "Afficher les statistiques en JSON" ascii
		$py_str_3 = "===STATS_START===" ascii
		$py_str_4 = "Error processing history for {browser_name}/{profile_name}" ascii
		$py_str_5 = "DECRYPT_FAILED" ascii
		$py_str_6 = "Fatal error in main: {str(e)}" ascii
	condition:
		5 of ($js_str_*) or
		4 of ($py_str_*)
}
