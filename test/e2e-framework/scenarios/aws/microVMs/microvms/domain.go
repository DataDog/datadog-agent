// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package microvms

import (
	"fmt"
	"net"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/pulumi/pulumi-libvirt/sdk/go/libvirt"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	microvmConfig "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/microVMs/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/microVMs/microvms/resources"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/microVMs/vmconfig"
)

const (
	dhcpEntriesTemplate = "<host mac='%s' name='%s' ip='%s'/>"
	sharedFSMountPoint  = "/opt/kernel-version-testing"
	maxDomainIDLength   = 64
	gdbPortRangeStart   = 4321
	gdbPortRangeEnd     = 4421
)

func getNextVMIP(ip *net.IP) net.IP {
	ipv4 := ip.To4()
	ipv4[3]++

	return ipv4
}

type Domain struct {
	resources.RecipeLibvirtDomainArgs
	domainID    string
	dhcpEntry   pulumi.StringOutput
	domainArgs  *libvirt.DomainArgs
	domainNamer namer.Namer
	ip          pulumi.StringOutput
	mac         pulumi.StringOutput
	lvDomain    *libvirt.Domain
	tag         string
	vmset       vmconfig.VMSet
	gdbPort     int
}

func generateDomainIdentifier(vcpu, memory int, vmsetTags, tag, arch string) string {
	// The domain id should always begin with 'arch'-'tag'-'vmsetTags'. This order
	// is expected in the consumers of this framework
	return fmt.Sprintf("%s-%s-%s-ddvm-%d-%d", arch, tag, vmsetTags, vcpu, memory)
}
func generateNewUnicastMac(e config.Env, domainID string) (pulumi.StringOutput, error) {
	r := utils.NewRandomGenerator(e, domainID)

	pulumiRandStr, err := r.RandomString(domainID, 6, true)
	if err != nil {
		return pulumi.StringOutput{}, err
	}

	macAddr := pulumiRandStr.Result.ApplyT(func(randStr string) string {
		buf := []byte(randStr)

		// Set LSB bit of MSB byte to 0
		// This denotes unicast mac address
		buf[0] &= 0xfe

		return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", buf[0], buf[1], buf[2], buf[3], buf[4], buf[5])
	}).(pulumi.StringOutput)

	return macAddr, nil
}

func generateMACAddress(e config.Env, domainID string) (pulumi.StringOutput, error) {
	mac, err := generateNewUnicastMac(e, domainID)
	if err != nil {
		return mac, err
	}

	return mac, err
}

func generateDHCPEntry(mac pulumi.StringOutput, ip pulumi.StringOutput, domainID string) pulumi.StringOutput {
	return pulumi.Sprintf(dhcpEntriesTemplate, mac, domainID, ip)
}

func getCPUTuneXML(vmcpus, hostCPUSet, cpuCount int) (string, int) {
	var vcpuMap []string

	if cpuCount == 0 {
		return "", 0
	}

	for i := 0; i < vmcpus; i++ {
		vcpuMap = append(vcpuMap, fmt.Sprintf("<vcpupin vcpu='%d' cpuset='%d'/>", i, hostCPUSet))
		hostCPUSet++
		if hostCPUSet >= cpuCount {
			// start from cpu 1, since we want to leave cpu 0 for the system
			hostCPUSet = 1
		}
	}

	return fmt.Sprintf("<cputune>%s</cputune>", strings.Join(vcpuMap, "\n")), hostCPUSet
}

func newDomainConfiguration(e config.Env, set *vmconfig.VMSet, vcpu, memory, gdbPort int, kernel vmconfig.Kernel, cputune string) (*Domain, error) {
	var err error

	domain := new(Domain)

	setTags := strings.Join(set.Tags, "-")
	domain.domainID = generateDomainIdentifier(vcpu, memory, setTags, kernel.Tag, set.Arch)
	if len(domain.domainID) >= maxDomainIDLength {
		// Apparently dnsmasq silently ignores entries with names longer than 63 characters, so static IPs don't get assigned correctly
		// and we can't connect to the VMs. We check for that case and fail loudly here instead.
		return nil, fmt.Errorf("%s domain ID length exceeds 63 characters, this can cause problems with some libvirt components", domain.domainID)
	}

	domain.domainNamer = libvirtResourceNamer(e.Ctx(), domain.domainID)
	domain.tag = kernel.Tag
	// copy the vmset tag. The pointer refers to
	// a local variable and can change causing an incorrect mapping
	domain.vmset = *set

	domain.mac, err = generateMACAddress(e, domain.domainID)
	if err != nil {
		return nil, err
	}

	rc := resources.NewResourceCollection(set.Recipe)
	domain.RecipeLibvirtDomainArgs.Resources = rc
	domain.RecipeLibvirtDomainArgs.Vcpu = vcpu
	domain.RecipeLibvirtDomainArgs.Memory = memory
	domain.RecipeLibvirtDomainArgs.ConsoleType = set.ConsoleType
	domain.RecipeLibvirtDomainArgs.KernelPath = filepath.Join(GetWorkingDirectory(set.Arch), "kernel-packages", kernel.Dir, "bzImage")

	domainName := libvirtResourceName(e.Ctx().Stack(), domain.domainID)
	varstore := filepath.Join(GetWorkingDirectory(set.Arch), fmt.Sprintf("varstore.%s", domainName))
	efi := filepath.Join(GetWorkingDirectory(set.Arch), "efi.fd")

	// OS-dependent settings
	var hypervisor string
	var commandLine pulumi.StringInput = pulumi.String("")
	var hostOS string
	if set.Arch == LocalVMSet {
		hostOS = runtime.GOOS
	} else {
		hostOS = "linux" // Remote VMs are always on Linux hosts
	}

	qemuArgs := make(map[string]pulumi.StringInput)
	if gdbPort != 0 {
		qemuArgs["-gdb"] = pulumi.Sprintf("tcp:127.0.0.1:%d", gdbPort)
		domain.gdbPort = gdbPort
	}

	var driver string
	if hostOS == "linux" {
		hypervisor = "kvm"
		driver = "<driver name=\"qemu\" type=\"qcow2\" io=\"io_uring\"/>"
	} else if hostOS == "darwin" {
		hypervisor = "hvf"
		// We have to use QEMU network devices because libvirt does not support the macOS
		// network devices.
		netID := libvirtResourceName(domainName, "netdev")

		qemuArgs["-netdev"] = pulumi.Sprintf("vmnet-shared,id=%s", netID)
		// Important: use virtio-net-pci instead of virtio-net-device so that the guest has a PCI
		// device and that information can be used by udev to rename the device, instead of having eth0.
		// This makes the naming consistent across different execution environments and avoids
		// problems (for example, DHCP is configured for interfaces starting with en*, so
		// if we had eth0 we wouldn't have a network connection)
		// Also, configure the PCI address as 17 so that we don't have conflicts with other libvirt controlled devices
		qemuArgs["-device"] = pulumi.Sprintf("virtio-net-pci,netdev=%s,mac=%s,addr=17", netID, domain.mac)
		driver = "<driver name=\"qemu\" type=\"qcow2\"/>"
	}

	for k, v := range qemuArgs {
		commandLine = pulumi.Sprintf("%s\n<arg value='%s' />", commandLine, k)
		commandLine = pulumi.Sprintf("%s\n<arg value='%s' />", commandLine, v)
	}

	domain.RecipeLibvirtDomainArgs.Xls = rc.GetDomainXLS(
		map[string]pulumi.StringInput{
			resources.SharedFSMount: pulumi.String(sharedFSMountPoint),
			resources.DomainID:      pulumi.String(domain.domainID),
			resources.MACAddress:    domain.mac,
			resources.Nvram:         pulumi.String(varstore),
			resources.Efi:           pulumi.String(efi),
			resources.VCPU:          pulumi.Sprintf("%d", vcpu),
			resources.CPUTune:       pulumi.String(cputune),
			resources.Hypervisor:    pulumi.String(hypervisor),
			resources.CommandLine:   commandLine,
			resources.DiskDriver:    pulumi.String(driver),
		},
	)
	domain.RecipeLibvirtDomainArgs.Machine = set.Machine
	domain.RecipeLibvirtDomainArgs.ExtraKernelParams = kernel.ExtraParams
	domain.RecipeLibvirtDomainArgs.DomainName = domainName

	return domain, nil
}

// We create a final overlay here so that each VM has its own unique writable disk.
// At this stage the chain of images is as follows: base-image -> overlay-1
// After this function we have: base-image -> overlay-1 -> overlay-2
// We have to do this because we have as many overlay-1's as the number of unique base images.
// However, we may want multiple VMs booted from the same underlying filesystem. To support this
// case we create a final overlay-2 for each VM to boot from.
func setupDomainVolume(ctx *pulumi.Context, providerFn LibvirtProviderFn, depends []pulumi.Resource, baseVolumeID, poolName, resourceName string) (*libvirt.Volume, error) {
	provider, err := providerFn()
	if err != nil {
		return nil, err
	}

	volume, err := libvirt.NewVolume(ctx, resourceName, &libvirt.VolumeArgs{
		BaseVolumeId: pulumi.String(baseVolumeID),
		Pool:         pulumi.String(poolName),
		Format:       pulumi.String("qcow2"),
	}, pulumi.Provider(provider), pulumi.DependsOn(depends))
	if err != nil {
		return nil, err
	}

	return volume, nil
}

func getVolumeDiskTarget(isRootVolume bool, lastDisk string) string {
	if isRootVolume {
		return "/dev/vda"
	}

	return fmt.Sprintf("/dev/vd%c", rune(int(lastDisk[len(lastDisk)-1])+1))
}

// isPortFree checks if a given TCP port on localhost in free
func isPortFree(port int) bool {
	address := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", address, 1*time.Second)
	if err != nil {
		// If there's an error connecting, we assume the port is free
		return true
	}

	conn.Close()
	// If connection was successful, port is in use
	return false
}

func GenerateDomainConfigurationsForVMSet(e config.Env, providerFn LibvirtProviderFn, depends []pulumi.Resource, set *vmconfig.VMSet, fs *LibvirtFilesystem, cpuSetStart int) ([]*Domain, int, error) {
	var domains []*Domain
	var cpuTuneXML string

	commonEnv, err := config.NewCommonEnvironment(e.Ctx())
	if err != nil {
		return nil, 0, err
	}
	m := microvmConfig.NewMicroVMConfig(commonEnv)
	setupGDB := m.GetBoolWithDefault(m.MicroVMConfig, microvmConfig.DDMicroVMSetupGDB, false) && set.Arch == LocalVMSet
	gdbPort := gdbPortRangeStart

	for _, vcpu := range set.VCpu {
		for _, memory := range set.Memory {
			for _, kernel := range set.Kernels {
				cpuTuneXML, cpuSetStart = getCPUTuneXML(vcpu, cpuSetStart, set.VMHost.AvailableCPUs)

				domainPort := 0
				if setupGDB {
					for port := gdbPort; port < gdbPortRangeEnd; port++ {
						if isPortFree(port) {
							domainPort = port
							break
						}
					}

					if domainPort == 0 {
						return nil, 0, fmt.Errorf("could not find free port in range [%d,%d] for gdb server", gdbPortRangeStart, gdbPortRangeEnd)
					}

					gdbPort = domainPort + 1 // evaluate another port in the next iteration
				}

				domain, err := newDomainConfiguration(e, set, vcpu, memory, domainPort, kernel, cpuTuneXML)
				if err != nil {
					return []*Domain{}, 0, err
				}

				// setup volume to be used by this domain
				libvirtVolumes := fs.baseVolumeMap[kernel.Tag]
				lastDisk := getVolumeDiskTarget(true, "")
				for _, vol := range libvirtVolumes {
					lastDisk = getVolumeDiskTarget(vol.Mountpoint() == RootMountpoint, lastDisk)
					rootVolume, err := setupDomainVolume(
						e.Ctx(),
						providerFn,
						depends,
						vol.Key(),
						vol.Pool().Name(),
						// adding the full domain ID causes the length of the resource name
						// to go beyond the maximum size allowed by pulumi.
						vol.FullResourceName("overlay", kernel.Tag, fmt.Sprintf("%d-%d", vcpu, memory)),
					)
					if err != nil {
						return []*Domain{}, 0, err
					}
					domain.Disks = append(domain.Disks, resources.DomainDisk{
						VolumeID:   pulumi.StringPtrInput(rootVolume.ID()),
						Target:     lastDisk,
						Mountpoint: vol.Mountpoint(),
					})
				}

				domain.domainArgs, err = domain.RecipeLibvirtDomainArgs.Resources.GetLibvirtDomainArgs(
					&domain.RecipeLibvirtDomainArgs,
				)
				if err != nil {
					return []*Domain{}, 0, fmt.Errorf("failed to setup domain arguments for %s: %v", domain.domainID, err)
				}

				domains = append(domains, domain)
			}
		}
	}

	return domains, cpuSetStart, nil

}
