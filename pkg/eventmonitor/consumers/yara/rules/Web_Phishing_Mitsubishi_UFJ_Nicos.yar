rule Web_Phishing_Mitsubishi_UFJ_Nicos {
    meta:
        description = "Detects phishing pages impersonating the Mitsubishi UFJ NICOS card brand selection and login portal."
        author = "Serhii Kocherhan"
        date = "2026-09-24"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "92daf7b5-28d5-4621-b763-c7bf7367ffc5"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "51be0ca44ecab9e9954987569c1283d6"

    strings:
        // Corporate Identity 
        $s_corp = { 95 b7 e6 98 c4 e3 83 cb e3 82 b3 e3 82 b9 } 

        // Specific Brand Asset Paths
        $s_asset_mufg = "./sl/select_logo_mufg.png" ascii wide
        $s_asset_dc = "./sl/select_logo_dc.png" ascii wide
        $s_asset_nicos = "./sl/select_logo_nicos.png" ascii wide

        // Login Action Parameters
        $s_login_type3 = "login?type=3" ascii wide
        $s_login_type4 = "login?type=4" ascii wide

    condition:
        filesize < 200KB and (
            $s_corp or
            (2 of ($s_asset*) and any of ($s_login*))
        )
}