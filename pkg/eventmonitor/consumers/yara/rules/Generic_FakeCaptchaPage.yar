rule Generic_FakeCaptchaPage {
	meta:
		author = "Still"
		component_name = "N/A"
		date = "2024-10-04"
		description = "attempts to match strings found in JavaScript/HTML used in captcha-styled malware delivery websites"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_reference_md5 = "BBA238F9275043DCD71F4FD681A1D8D5"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "e4e0690f-eb92-4f32-a4af-d78918898c9e"
	strings:
		$str_1 = "recaptchaPopup" ascii fullword
		$str_2 = "verifyButton" ascii fullword
		$str_3 = "const tempTextArea" ascii fullword
		$str_4 = "Verify You Are Human" ascii fullword
		$str_5 = "CTRL + V" ascii fullword
	condition:
		3 of them
}
