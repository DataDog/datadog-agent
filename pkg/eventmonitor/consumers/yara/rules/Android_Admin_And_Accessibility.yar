rule Android_Admin_And_Accessibility
{
	meta:
		author = "Buga :3"
		date = "2024-06-26"
		description = "This detects apps which request access to both device admin and the Android accessibility suite."
		yarahub_uuid = "6d191b29-9dc4-4969-97a4-9db44471a91f"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_reference_md5 = "1b90070f260dd28c37d09ed09a993286"

	    strings:
        $permission1 = "android.permission.BIND_ACCESSIBILITY_SERVICE"
        $permission2 = "android.permission.BIND_DEVICE_ADMIN"

    condition:
        $permission1 and $permission2
}