rule WEBSHELL_PHP_gov_uploader
{
    meta:
        description     = "PHP upload/rename webshell using 'gov'-prefixed variable obfuscation. Shipped as log.db beside a three-line index.php loader inside a fake WordPress plugin archive; the loader includes it so the only .php in the archive looks harmless."
        author          = "Efrain Gutierrez"
        date            = "2026-09-17"

        yarahub_uuid              = "c0e215cc-b1b8-4389-8e96-6f7c5e311267"
        yarahub_reference_md5     = "b1c85b559053772b327663df36968933"
        yarahub_license           = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"

        hash_sha256     = "79a40766ab58027817db28412d600be9f3e64f8e24133562cc3aacb500ec720a"
        hash_md5        = "b1c85b559053772b327663df36968933"
        loader_sha256   = "10cf2e3f52b5daee5ee47fb639a17003f019a3d8f9d0c0fc8fdfbcc71ff88453"
        archive_sha256  = "84e43d5a02b0e96fb36661cec1882b00140ca65c9160467171d5381a1d9ca3ba"
        note            = "Undetected by 61 AV engines, ClamAV and all YARAify community rules as of 2026-09-17. Observed in the wild since ~2021."

    strings:
        // Builder artefact: every variable is "gov" followed by random alphanumerics.
        $gov = /\$gov[A-Za-z0-9]{4,24}/

        // Function names assembled from reversed and split fragments so that no
        // recognisable call appears in the source.
        $obf_rev_move = "aolpu_evom"        ascii   // strrev -> move_uploa(ded_file)
        $obf_rev_name = "'eman'"            ascii   // strrev -> name
        $obf_split_fg = "'file_g'"          ascii   // -> file_g(et_contents)
        $obf_split_b6 = "'base64_'"         ascii
        $obf_b64_call = " . base64_decode(" ascii

        // Configuration held as double base64: "AES128" and its key material.
        $cfg_aes = "UVVWVE1UST0="                       ascii
        $cfg_k1  = "TVRaaFkyRmpZekExWVdGbVlXWTI"        ascii
        $cfg_k2  = "YjJKMWFHRnZjbkJtTlhWM05EUmtkbTl4"   ascii

        // Operator-facing text. Carried across builds even when keys change.
        $ui_1 = "Both Original Name and New Name must be provided." ascii
        $ui_2 = "Failed to upload file. Debug Info:"                ascii
        $ui_3 = "Failed to rename file. Please check permissions or file path." ascii
        $ui_4 = "Failed to create directory:"                       ascii

    condition:
        filesize < 100KB
        and uint32be(0) == 0x3C3F7068      // "<?ph"

        and (
            // Obfuscation scheme plus the naming artefact.
            ( #gov >= 4 and 2 of ($obf_*) )

            // Or the operator text plus either marker, for a rebuild that
            // renames its variables or rotates its keys.
            or ( 2 of ($ui_*) and ( #gov >= 2 or 1 of ($obf_*) ) )

            // Or the embedded key material, which identifies this build exactly.
            or ( $cfg_aes and 1 of ($cfg_k*) )
        )
}
