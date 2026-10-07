rule win_vidar_a_a901 {

    meta:
        author                    = "Johannes Bader"
        date                      = "2023-03-30"
        description               = "detect unpacked Vidar samples"
        hash_md5                  = "ed4ddd89e6ab5211cd7fdbfe51d9576b"
        hash_sha1                 = "7b6beb9870646bc50b10014536ed3bb088a2e3de"
        hash_sha256               = "352f8e45cd6085eea17fffeeef91251192ceaf494336460cc888bbdd0051ec71"
        malpedia_family           = "win.vidar"
        tlp                       = "TLP:WHITE"
        version                   = "v1.0"
        yarahub_author_email      = "yara@bin.re"
        yarahub_author_twitter    = "@viql"
        yarahub_license           = "CC BY-SA 4.0"
        yarahub_reference_md5     = "ed4ddd89e6ab5211cd7fdbfe51d9576b"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_uuid              = "a901638e-af37-42a0-a4c5-8f20d4a7e148"

    strings:
        $leet_sleep  = {6A 01 FF D6 6A 03 FF D6 6A 03 FF D6 6A 07 FF D6}

        $wallets_01 = "Enkrypt"
        $wallets_02 = "Braavos"
        $wallets_03 = "Exodus Web3 Wallet"
        $wallets_04 = "Trust Wallet"
        $wallets_05 = "Tronium"
        $wallets_06 = "Opera Wallet"
        $wallets_07 = "OKX Web3 Wallet"
        $wallets_08 = "Sender"
        $wallets_09 = "Hashpack"
        $wallets_10 = "Eternl"
        $wallets_11 = "GeroWallet"
        $wallets_12 = "Pontem Wallet"
        $wallets_13 = "Martian Wallet"
        $wallets_14 = "Finnie"
        $wallets_15 = "Leap Terra"
        $wallets_16 = "Microsoft AutoFill"
        $wallets_17 = "Bitwarden"
        $wallets_18 = "KeePass Tusk"
        $wallets_19 = "KeePassXC-Browser"

        $telegram_1 = "shortcuts-default.json"
        $telegram_2 = "shortcuts-custom.json"
        $telegram_3 = "settingss"
        $telegram_4 = "prefix"
        $telegram_5 = "countries"
        $telegram_6 = "usertag"

        $scp = "Software\\Martin Prikryl\\WinSCP 2\\Configuration" wide

    condition:
        uint16(0) == 0x5A4D and
        (
            #leet_sleep > 10 and
            (16 of ($wallets_*) and all of ($telegram_*) and $scp)
        )
}
