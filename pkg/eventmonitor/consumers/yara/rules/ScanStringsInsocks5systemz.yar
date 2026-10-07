rule ScanStringsInsocks5systemz {
	meta:
		description = "Scans presence of the found strings using the in-house brute force method"
		author = "Byambaa@pubcert.mn"
		date = "2024-10-01"
        	yarahub_uuid = "cd061b79-9264-480a-bda6-2242046143d5"
        	yarahub_license = "CC0 1.0"
        	yarahub_rule_matching_tlp = "TLP:WHITE"
        	yarahub_rule_sharing_tlp = "TLP:WHITE"
        	yarahub_reference_md5 = "73875E9DA68182B09BC6A7FAAFFF67D8"
	strings:
		$string0 = "Mozilla/5.0 (Windows; U; MSIE 9.0; Windows NT 9.0; en-US)"
		$string1 = "$*@@@*$@@@$ *@@* $@@($*)@-$*@@$-*@@$*-@@(*$)@-*$@@*-$@@*$-@@-* $@-$ *@* $-@$ *-@$ -*@*- $@($ *)(* $)U"
	condition:
		any of them
	}
