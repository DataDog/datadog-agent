// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractSimpleGPUName(t *testing.T) {
	tests := []struct {
		name     string
		gpuName  ResourceGPU
		found    bool
		expected string
	}{
		{
			name:     "known gpu resource",
			gpuName:  GpuNvidiaGeneric,
			found:    true,
			expected: "nvidia",
		},
		{
			name:     "known dra gpu resource",
			gpuName:  GpuNvidiaDRA,
			found:    true,
			expected: "nvidia",
		},
		{
			name:     "unknown gpu resource",
			gpuName:  ResourceGPU("cpu"),
			found:    false,
			expected: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, found := ExtractSimpleGPUName(test.gpuName)
			assert.Equal(t, test.found, found)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestIsNvidiaKubernetesResource(t *testing.T) {
	tests := []struct {
		name         string
		resourceName string
		expected     bool
	}{
		{
			name:         "device plugin generic GPU",
			resourceName: string(GpuNvidiaGeneric),
			expected:     true,
		},
		{
			name:         "DRA GPU driver",
			resourceName: string(GpuNvidiaDRA),
			expected:     true,
		},
		{
			name:         "MIG GPU",
			resourceName: "nvidia.com/mig-3g.20gb",
			expected:     true,
		},
		{
			name:         "non-NVIDIA resource",
			resourceName: string(GpuAMD),
			expected:     false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, IsNvidiaKubernetesResource(test.resourceName))
		})
	}
}

func TestExtractGPUType(t *testing.T) {
	tests := []struct {
		deviceName string
		expected   string
	}{
		// instances: g4dn.12xlarge, g4dn.16xlarge, g4dn.2xlarge, g4dn.4xlarge, g4dn.8xlarge, g4dn.metal, g4dn.xlarge, standard_nc16as_t4_v3, standard_nc4as_t4_v3, standard_nc64as_t4_v3, standard_nc8as_t4_v3
		{deviceName: "Tesla T4", expected: "t4"},
		// instances: g4dn.12xlarge, g4dn.16xlarge, g4dn.2xlarge, g4dn.4xlarge, g4dn.8xlarge, g4dn.metal, g4dn.xlarge, standard_nc16as_t4_v3, standard_nc4as_t4_v3, standard_nc64as_t4_v3, standard_nc8as_t4_v3
		{deviceName: "Tesla_T4", expected: "t4"},
		{deviceName: "Tesla-T4", expected: "t4"},
		// instances: g5g.16xlarge, g5g.2xlarge, g5g.4xlarge, g5g.8xlarge, g5g.metal, g5g.xlarge
		{deviceName: "NVIDIA T4G", expected: "t4g"},
		// instances: g5g.16xlarge, g5g.2xlarge, g5g.4xlarge, g5g.8xlarge, g5g.metal, g5g.xlarge
		{deviceName: "NVIDIA_T4G", expected: "t4g"},
		// instances: standard_nc16ads_a10_v4, standard_nc32ads_a10_v4, standard_nc8ads_a10_v4, standard_nv12ads_a10_v5, standard_nv18ads_a10_v5, standard_nv36adms_a10_v5, standard_nv36ads_a10_v5, standard_nv6ads_a10_v5, standard_nv72ads_a10_v5
		{deviceName: "NVIDIA A10", expected: "a10"},
		// instances: standard_nc16ads_a10_v4, standard_nc32ads_a10_v4, standard_nc8ads_a10_v4, standard_nv12ads_a10_v5, standard_nv18ads_a10_v5, standard_nv36adms_a10_v5, standard_nv36ads_a10_v5, standard_nv6ads_a10_v5, standard_nv72ads_a10_v5
		{deviceName: "NVIDIA_A10", expected: "a10"},
		// instances: NVadsA10_v5 family
		{deviceName: "NVIDIA A10-4Q", expected: "a10"},
		// instances: g5.12xlarge, g5.16xlarge, g5.24xlarge, g5.2xlarge, g5.48xlarge, g5.4xlarge, g5.8xlarge, g5.xlarge
		{deviceName: "NVIDIA A10G", expected: "a10g"},
		// instances: g5.12xlarge, g5.16xlarge, g5.24xlarge, g5.2xlarge, g5.48xlarge, g5.4xlarge, g5.8xlarge, g5.xlarge
		{deviceName: "NVIDIA_A10G", expected: "a10g"},
		// instances: a2-highgpu-1g, a2-highgpu-2g, a2-highgpu-4g, a2-highgpu-8g, a2-megagpu-16g, a2-ultragpu-1g, a2-ultragpu-2g, a2-ultragpu-4g, a2-ultragpu-8g, p4d.24xlarge, p4de.24xlarge, standard_nc24ads_a100_v4, standard_nc48ads_a100_v4, standard_nc96ads_a100_v4, standard_nd96amsr_a100_v4, standard_nd96asr_v4
		{deviceName: "NVIDIA A100-SXM4-40GB", expected: "a100"},
		// instances: a2-highgpu-1g, a2-highgpu-2g, a2-highgpu-4g, a2-highgpu-8g, a2-megagpu-16g, a2-ultragpu-1g, a2-ultragpu-2g, a2-ultragpu-4g, a2-ultragpu-8g, p4d.24xlarge, p4de.24xlarge, standard_nc24ads_a100_v4, standard_nc48ads_a100_v4, standard_nc96ads_a100_v4, standard_nd96amsr_a100_v4, standard_nd96asr_v4
		{deviceName: "NVIDIA_A100-SXM4-40GB", expected: "a100"},
		// Kubernetes GPU Feature Discovery exposes product names as label-safe values with dashes instead of spaces.
		{deviceName: "NVIDIA-A100-SXM4-40GB", expected: "a100"},
		{deviceName: "NVIDIA A100 80GB PCIe MIG 3g.40gb", expected: "a100"},
		{deviceName: "NVIDIA-A100-80GB-PCIe-MIG-3g.40gb-SHARED", expected: "a100"},
		// instances: a3-edgegpu-8g, a3-edgegpu-8g-nolssd, a3-highgpu-1g, a3-highgpu-2g, a3-highgpu-4g, a3-highgpu-8g, a3-megagpu-8g, p5.48xlarge, p5.4xlarge, standard_nc40ads_h100_v5, standard_nc80adis_h100_v5, standard_ncc40ads_h100_v5, standard_nd96isr_h100_v5
		{deviceName: "NVIDIA H100-PCIE", expected: "h100"},
		// instances: a3-edgegpu-8g, a3-edgegpu-8g-nolssd, a3-highgpu-1g, a3-highgpu-2g, a3-highgpu-4g, a3-highgpu-8g, a3-megagpu-8g, p5.48xlarge, p5.4xlarge, standard_nc40ads_h100_v5, standard_nc80adis_h100_v5, standard_ncc40ads_h100_v5, standard_nd96isr_h100_v5
		{deviceName: "NVIDIA_H100-PCIE", expected: "h100"},
		{deviceName: "NVIDIA H100 NVL MIG 3g.47gb", expected: "h100"},
		{deviceName: "NVIDIA-H100-NVL-MIG-3g.47gb", expected: "h100"},
		// instances: a3-ultragpu-8g, a3-ultragpu-8g-nolssd, p5en.48xlarge, standard_nd96isr_h200_v5
		{deviceName: "NVIDIA H200", expected: "h200"},
		// instances: a3-ultragpu-8g, a3-ultragpu-8g-nolssd, p5en.48xlarge, standard_nd96isr_h200_v5
		{deviceName: "NVIDIA_H200", expected: "h200"},
		// instances: p3.16xlarge, p3.2xlarge, p3.8xlarge, p3dn.24xlarge, standard_nc12s_v3, standard_nc24rs_v3, standard_nc24s_v3, standard_nc6s_v3, standard_nd40rs_v2
		{deviceName: "NVIDIA V100-32GB", expected: "v100"},
		// instances: p3.16xlarge, p3.2xlarge, p3.8xlarge, p3dn.24xlarge, standard_nc12s_v3, standard_nc24rs_v3, standard_nc24s_v3, standard_nc6s_v3, standard_nd40rs_v2
		{deviceName: "NVIDIA_V100-32GB", expected: "v100"},
		// instances: g2-standard-12, g2-standard-16, g2-standard-24, g2-standard-32, g2-standard-4, g2-standard-48, g2-standard-8, g2-standard-96, g6.12xlarge, g6.16xlarge, g6.24xlarge, g6.2xlarge, g6.48xlarge, g6.4xlarge, g6.8xlarge, g6.xlarge, g6f.2xlarge, g6f.4xlarge, g6f.large, g6f.xlarge, gr6.4xlarge, gr6.8xlarge, gr6f.4xlarge
		{deviceName: "NVIDIA L4", expected: "l4"},
		// instances: g2-standard-12, g2-standard-16, g2-standard-24, g2-standard-32, g2-standard-4, g2-standard-48, g2-standard-8, g2-standard-96, g6.12xlarge, g6.16xlarge, g6.24xlarge, g6.2xlarge, g6.48xlarge, g6.4xlarge, g6.8xlarge, g6.xlarge, g6f.2xlarge, g6f.4xlarge, g6f.large, g6f.xlarge, gr6.4xlarge, gr6.8xlarge, gr6f.4xlarge
		{deviceName: "NVIDIA_L4", expected: "l4"},
		{deviceName: " NVIDIA L4 ", expected: "l4"},
		{deviceName: "NVIDIA-L4", expected: "l4"},
		{deviceName: "L4", expected: ""},
		{deviceName: "l4", expected: ""},
		{deviceName: "NVIDIA: L4", expected: "l4"},
		{deviceName: "NVIDIA   L4", expected: "l4"},
		{deviceName: "NVIDIA__L4", expected: "l4"},
		{deviceName: "NVIDIA--L4", expected: "l4"},
		{deviceName: "NVIDIA L4 24GB", expected: "l4"},
		{deviceName: "NVIDIA L4-24GB", expected: "l4"},
		{deviceName: "NVIDIA L4 (rev.2)", expected: "l4"},
		{deviceName: "\"NVIDIA L4\"", expected: "l4"},
		{deviceName: "'NVIDIA L4'", expected: "l4"},
		{deviceName: "NVIDIA GeForce-RTX-3090", expected: "rtx_3090"},
		{deviceName: "NVIDIA GeForce RTX_3090", expected: "rtx_3090"},
		{deviceName: "NVIDIA GeForce   RTX 3090", expected: "rtx_3090"},
		// instances: g6e.12xlarge, g6e.16xlarge, g6e.24xlarge, g6e.2xlarge, g6e.48xlarge, g6e.4xlarge, g6e.8xlarge, g6e.xlarge
		{deviceName: "NVIDIA L40S", expected: "l40s"},
		// instances: g6e.12xlarge, g6e.16xlarge, g6e.24xlarge, g6e.2xlarge, g6e.48xlarge, g6e.4xlarge, g6e.8xlarge, g6e.xlarge
		{deviceName: "NVIDIA_L40S", expected: "l40s"},
		// instances: standard_nv12s_v2, standard_nv12s_v3, standard_nv24s_v2, standard_nv24s_v3, standard_nv48s_v3, standard_nv6s_v2
		{deviceName: "Tesla M60", expected: "m60"},
		// instances: standard_nv12s_v2, standard_nv12s_v3, standard_nv24s_v2, standard_nv24s_v3, standard_nv48s_v3, standard_nv6s_v2
		{deviceName: "Tesla_M60", expected: "m60"},
		// instances: p6-b200.48xlarge
		{deviceName: "NVIDIA B200-96GB", expected: "b200"},
		// instances: p6-b200.48xlarge
		{deviceName: "NVIDIA_B200-96GB", expected: "b200"},
		{deviceName: "NVIDIA RTX A6000", expected: "rtx_a6000"},
		{deviceName: "NVIDIA_RTX_A6000", expected: "rtx_a6000"},
		{deviceName: "NVIDIA-RTX-A6000", expected: "rtx_a6000"},
		{deviceName: "NVIDIA RTX 6000 Ada Generation", expected: "rtx_6000"},
		{deviceName: "NVIDIA_RTX_6000_Ada_Generation", expected: "rtx_6000"},
		{deviceName: "NVIDIA-RTX-6000-Ada-Generation", expected: "rtx_6000"},
		{deviceName: "NVIDIA GeForce RTX 3090", expected: "rtx_3090"},
		{deviceName: "NVIDIA_GeForce_RTX_3090", expected: "rtx_3090"},
		{deviceName: "NVIDIA-GeForce-RTX-3090", expected: "rtx_3090"},
		{deviceName: "NVIDIA GeForce RTX 4090", expected: "rtx_4090"},
		{deviceName: "NVIDIA_GeForce_RTX_4090", expected: "rtx_4090"},
		{deviceName: "NVIDIA-GeForce-RTX-4090", expected: "rtx_4090"},
		{deviceName: "", expected: ""},
		{deviceName: "Unknown GPU", expected: ""},
		{deviceName: "Unknown_GPU", expected: ""},
		{deviceName: "nViDiA a100", expected: "a100"},
	}

	for _, tt := range tests {
		t.Run(tt.deviceName, func(t *testing.T) {
			assert.Equal(t, tt.expected, ExtractGPUType(tt.deviceName))
		})
	}
}

func TestGFDLabelToGPUDeviceName(t *testing.T) {
	// NVML-style display names; GFD round-trip test uses strings.ReplaceAll(name, " ", "-") as the label.
	deviceNames := []string{
		"Tesla T4",
		"NVIDIA T4G",
		"NVIDIA A10",
		"NVIDIA A10-4Q",
		"NVIDIA A10-24Q",
		"NVIDIA A10G",
		"NVIDIA A100-SXM4-40GB",
		"NVIDIA A100 80GB PCIe",
		"NVIDIA A100-SXM4-40GB MIG 1g.5gb",
		"NVIDIA A100-SXM4-40GB MIG 2g.10gb",
		"NVIDIA A100-SXM4-40GB MIG 3g.20gb",
		"NVIDIA H100 NVL",
		"NVIDIA H100 NVL MIG 3g.47gb",
		"NVIDIA H100 80GB HBM3",
		"NVIDIA H200",
		"NVIDIA V100-32GB",
		"NVIDIA L4",
		"NVIDIA L40S",
		"NVIDIA RTX PRO 6000 Blackwell Server Edition",
		"NVIDIA B300 SXM6 AC",
		"NVIDIA B200",
		"Tesla M60",
		"NVIDIA RTX A6000",
		"NVIDIA RTX 6000 Ada Generation",
	}

	for _, deviceName := range deviceNames {
		t.Run(deviceName, func(t *testing.T) {
			gfdLabel := strings.ReplaceAll(deviceName, " ", "-")
			assert.Equal(t, NormalizeGPUDeviceName(deviceName), NormalizeGPUDeviceName(GFDLabelToGPUDeviceName(gfdLabel)))
		})
	}
}
