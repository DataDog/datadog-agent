rule LIN_Sample_Unique_c5e444d3 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "c5e444d33dae5bdb4b35c6ffc3d8bbd42c05122677d50b5177c3aef683da1124.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "e8b1493b-94c0-4956-8c31-27346d74f0c9"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "b2a6ff9b6defb4d5f0e7f51131c41329"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = " run 'conda deactivate' (repeat until the env var/prompt clears) and re-run this installer. This project must NOT be installed inside conda.\")" ascii wide
        $b = "            say \"    --gpu-memory-utilization $GPU_UTIL --max-model-len $MAX_LEN --max-num-seqs $MAX_SEQS --enforce-eager --port 8000\"" ascii wide
    condition:
        all of them
}
