rule LIN_Dropper_Unknown_Zsh_AES_SplitCmd {
    meta:
        description = "Dropper zsh : commandes eclatees en variables (md5/xxd/openssl/gunzip) + payload AES-128-CTR decode puis execute via /bin/zsh"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "Veille glissante 2026-09-18_08 / cc922fb8"
        confidence  = "high"
        yarahub_uuid = "26070961-0c49-4229-b5ea-ba00c9af956f"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5   = "58d6b34e0269a5f7b2e76be2c1d8f0f4"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $a = "_probe_salt" ascii wide
        $b = "print -r -- \"$_r\" | /bin/zsh" ascii wide
    condition:
        all of them
}
