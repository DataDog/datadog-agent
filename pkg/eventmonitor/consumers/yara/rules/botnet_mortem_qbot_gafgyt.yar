rule botnet_mortem_qbot_gafgyt
{
    meta:
        description = "Some strings that stand out from a publicly-available botnet source code (Mortem-qBot-Botnet-Src)"
        author = "cip"
        family = "Gafgyt"
        date = "2025-06-02"
        yarahub_uuid = "9475efad-e517-4aca-92ce-2e1419a5c809"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "334a50e61b94fd70971bee04d0a99a43"

    strings:
        $yakuza = "YakuzaBotnet"
        $scarface = "Scarface1337"

    condition:
        $yakuza or $scarface
}