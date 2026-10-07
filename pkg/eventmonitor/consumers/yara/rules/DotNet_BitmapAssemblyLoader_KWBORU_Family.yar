rule DotNet_BitmapAssemblyLoader_KWBORU_Family
{
    meta:
        author = "ShadowOpCode"
        description = "Detects the outer .NET loader template that reconstructs a managed assembly from bitmap RGB data and invokes it reflectively"
        date = "2026-09-28"
        version = "1.0"
        tlp = "CLEAR"
        malware_family = "KWBORU_BitmapAssemblyLoader"
        confidence = "medium"

        // YARAhub / YARAify metadata
        yarahub_uuid = "b9e4efc9-38a7-4004-9605-f306b21b4038"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "00000000000000000000000000000000"

    strings:
        $dotnet = "BSJB" ascii

        $helper1 = "GroupingProjectionExtractor" ascii
        $helper2 = "ComputeEpidemiologyMetrics" ascii
        $helper3 = "GammaFunction" ascii

        $reflect1 = "InvokeMember" ascii
        $reflect2 = "CreateInstance" ascii
        $reflect3 = "GetTypes" ascii
        $reflect4 = "GetObject" ascii

        $delimiter = "0x1A4F" ascii wide

    condition:
        uint16(0) == 0x5A4D and
        $dotnet and
        2 of ($helper*) and
        3 of ($reflect*) and
        $delimiter
}
