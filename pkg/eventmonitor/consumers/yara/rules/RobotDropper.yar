rule RobotDropper {
    meta:
        author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
        date = "2024-08-29"
        description = "Detects RobotDropper"
        yarahub_uuid = "0c9b4d1c-fa9e-4435-a7bb-954e7dd6d796"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "f89109ce397d50081ea28f31a8f61952"
        
    strings:   
        $MSI = "ProductCode" ascii
        $MSI2 = "ProductVersion" ascii
        
        $CustomAction = "CustomActionData" ascii
        $ButtonPressed = "BTN_PRESSED" ascii
        $RAR_Extraction = ".rar\" \"[APPDIR]\"" ascii
	$c2Path = "licenseUser.php" ascii

    condition:
        5 of them
}