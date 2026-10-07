rule Foxveil_Stage1_Zsh_ConstKey_AesCtr
{
    meta:
        yarahub_uuid = "db3f38e0-5187-4a37-b827-a3b5f2c944ee"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "3d62f3f526c946f718bd07cce9808435"
        description = "Foxveil ClickFix stage-1 zsh dropper: hex blobs -> md5(constant _kb) -> openssl AES-128-CTR IV 0 -> gunzip -> zsh, behind a decoy maintenance script"
        actor = "Foxveil"
        family = "AMOS ClickFix dropper"
        reference_sha256 = "6e753133e537264247a08587d87b30b8d9a96caea1d6cc92d41fc8058073b8f7"
        date = "2026-09-18"
        tlp = "CLEAR"

    strings:
        $shebang = "#!/bin/zsh"
        $kb      = "_kb=$(( ( ${#"
        $k       = "_k=$(printf '%s' \"$_kb\" | ${"
        $dec     = "enc -d -aes-128-ctr -K \"$_k\" -iv 00000000000000000000000000000000 | ${"
        $unhex   = "} -r -p | ${"
        $arr     = /_(blob|cfg)=\( "\$_[a-z_]+" "\$_[a-z_]+"/
        $hexvar  = /\n_[a-z_]{4,20}="[0-9a-f]{90,}"\n/

    condition:
        filesize < 32KB and $shebang at 0 and $dec and $k and
        2 of ($kb, $unhex, $arr) and #hexvar >= 3
}
