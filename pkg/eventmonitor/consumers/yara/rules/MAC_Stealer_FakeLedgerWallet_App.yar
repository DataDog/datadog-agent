rule MAC_Stealer_FakeLedgerWallet_App {
    meta:
        description = "Faux Ledger Wallet.app (macOS) - bundle imitant le logiciel Ledger, voleur de crypto a confirmer"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "misses_archive / f8d09bb7"
        confidence  = "medium"
        note        = "Structure .app imitant Ledger Wallet ; verifier la signature Apple et le binaire MacOS en sandbox"
        yarahub_uuid            = "87d104d8-3894-4da3-b6d7-cd43eea321ad"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "a79a20c46cbda66cbc93eae7f1e9173e"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "Ledger Wallet.app/Contents/MacOS/Ledger Wallet" ascii wide
        $b = "Ledger Wallet.app/Contents/_CodeSignature/" ascii wide
        $c = "ledger-live.css" ascii wide
    condition:
        uint32(0) == 0x04034b50 and $a and 1 of ($b, $c)
}
