rule APT_Bitter_PDB_Paths {
    
    meta:
        description = "Detects Bitter (T-APT-17) PDB Paths"
        author = "SECUINFRA Falcon Team (@SI_FalconTeam)"
        tlp = "WHITE"
        yarahub_uuid = "1f78e5ba-4c6c-4f14-9f43-78936d0ab687"
        yarahub_reference_md5 = "71e1cfb5e5a515cea2c3537b78325abf"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_author_twitter = "@SI_FalconTeam"
        reference = "https://www.secuinfra.com/en/techtalk/whatever-floats-your-boat-bitter-apt-continues-to-target-bangladesh"
        date = "2022-06-22"
        hash0 = "55901c2d5489d6ac5a0671971d29a31f4cdfa2e03d56e18c1585d78547a26396"

    strings:
        // Almond RAT
        $pdbPath0 = "C:\\Users\\Window 10 C\\Desktop\\COMPLETED WORK\\" ascii
        $pdbPath1 = "stdrcl\\stdrcl\\obj\\Release\\stdrcl.pdb"

        // found by Qi Anxin Threat Intellingence Center
        // reference: https://mp.weixin.qq.com/s/8j_rHA7gdMxY1_X8alj8Zg
        $pdbPath2 = "g:\\Projects\\cn_stinker_34318\\"
        $pdbPath3 = "renewedstink\\renewedstink\\obj\\Release\\stimulies.pdb"

    condition:
        uint16(0) == 0x5a4d
        and any of ($pdbPath*)
}