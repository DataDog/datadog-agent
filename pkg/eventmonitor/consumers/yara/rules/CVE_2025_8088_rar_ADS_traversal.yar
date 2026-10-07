rule CVE_2025_8088_rar_ADS_traversal {
	meta:
		description = "Detects CVE-2025-8088 WinRAR NTFS ADS path traversal exploitation"
		author = "Travis Green <travis.green@corelight.com>"
		reference = "https://www.welivesecurity.com/en/eset-research/update-winrar-tools-now-romcom-and-others-exploiting-zero-day-vulnerability/"
		date = "2025-08-11"
		version = "1.0"
		hash1 = "107f3d1fe28b67397d21a6acca5b6b35def1aeb62a67bc10109bd73d567f9806"
		tlp = "WHITE"
		yarahub_reference_md5 = "df9cfd04d8cda6df8f7263af54f9e5b1"
		yarahub_author_twitter = "@travisbgreen"
		yarahub_author_email = "travis.green@corelight.com"
		yarahub_reference_link = "https://travisgreen.net/2025/08/11/CVE-2025-8088.html"
		yarahub_uuid = "b9a882e6-efc0-4d67-afe5-ca1a42adbef4"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
	strings:
		$x1 = "STM" fullword ascii
		$x2 = "..\\\\" fullword ascii
		$x3 = /STM..\x3a[^\x00]*\x2e\x2e\x5c/ ascii
	condition:
		uint16(0) == 0x6152 and 3 of ($x*)
}
