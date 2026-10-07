rule win_xfiles_stealer_a8b373fb {

    meta:
        author                    = "Johannes Bader"
        date                      = "2022-04-15"
        description               = "detects XFiles-Stealer"
        hash                      = "d06072f959d895f2fc9a57f44bf6357596c5c3410e90dabe06b171161f37d690"
        hash2                     = "1ed070e0d33db9f159a576e6430c273c"
        malpedia_family           = "win.xfilesstealer"
        tlp                       = "TLP:WHITE"
        version                   = "v1.0"
        yarahub_author_email      = "yara@bin.re"
        yarahub_author_twitter    = "@viql"
        yarahub_license           = "CC BY-SA 4.0"
        yarahub_reference_md5     = "1ed070e0d33db9f159a576e6430c273c"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_uuid              = "a8b373fb-337a-4c3c-9387-78c294c8017d"

    strings:
        $ad_1 = "Telegram bot - @XFILESShop_Bot" wide
        $ad_2 = "Telegram support - @XFILES_Seller" wide

        $names_1 = "XFiles.Models.Yeti"
        $names_2 = "anti_vzlom_popki" // анти взлом попки
        $names_3 = "assType"
        $names_4 = "hackrjaw"

        $upload_1  = "zipx" wide
        $upload_2  = "user_id" wide
        $upload_3  = "passworlds_x" wide
        $upload_4  = "ip_x" wide
        $upload_5  = "cc_x" wide
        $upload_6  = "cookies_x" wide
        $upload_7  = "zip_x" wide
        $upload_8  = "contry_x" wide
        $upload_9  = "tag_x" wide
        $upload_10 = "piece" wide

    condition:
        uint16(0) == 0x5A4D and
        (
            all of ($ad_*) or
            all of ($names_*) or
            all of ($upload_*)
        )
}
