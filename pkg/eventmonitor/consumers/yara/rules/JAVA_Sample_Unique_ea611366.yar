rule JAVA_Sample_Unique_ea611366
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "ea6113660955c4b9796c1d22db9d9019ca1797c6707d040e8c11d44be3b921bc"
        yarahub_uuid = "6a7784fd-5344-4a23-8f65-208755e061f7"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "3cb85e1184f055ac1913f83dbdc2ab10"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "0client/xenon/imixin/IServerboundMovePlayerPacket" ascii
        $s1 = "IServerboundMovePlayerPacket.java" ascii
        $s2 = "setHasRot" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them
}
