import Foundation
import Virtualization

struct Stderr: TextOutputStream {
    mutating func write(_ string: String) {
        FileHandle.standardError.write(Data(string.utf8))
    }
}

func fail(_ message: String, code: Int32 = 1) -> Never {
    var stderr = Stderr()
    print("agent-sandbox-vz: \(message)", to: &stderr)
    exit(code)
}

struct Options {
    var command: String
    var disk: String?
    var seed: String?
    var efi: String?
    var serial: String?
    var aptCache: String?
    var mac: String?
    var cpus: Int = 2
    var memoryMiB: UInt64 = 4096
}

func parseOptions(_ args: [String]) -> Options {
    guard let command = args.first else {
        fail("usage: agent-sandbox-vz validate|start --disk PATH --seed PATH --efi PATH --serial PATH [--cpus N] [--memory-mib N]")
    }
    var options = Options(command: command)
    var index = 1
    while index < args.count {
        let key = args[index]
        guard index + 1 < args.count else { fail("missing value for \(key)") }
        let value = args[index + 1]
        switch key {
        case "--disk": options.disk = value
        case "--seed": options.seed = value
        case "--efi": options.efi = value
        case "--serial": options.serial = value
        case "--apt-cache": options.aptCache = value
        case "--mac": options.mac = value
        case "--cpus": options.cpus = Int(value) ?? options.cpus
        case "--memory-mib": options.memoryMiB = UInt64(value) ?? options.memoryMiB
        default: fail("unknown option \(key)")
        }
        index += 2
    }
    return options
}

func requireFile(_ path: String?, label: String) -> URL {
    guard let path = path else { fail("missing --\(label)") }
    let url = URL(fileURLWithPath: path)
    guard FileManager.default.fileExists(atPath: url.path) else { fail("\(label) does not exist: \(path)") }
    return url
}

func ensureParentDirectory(_ path: String?, label: String) -> URL {
    guard let path = path else { fail("missing --\(label)") }
    let url = URL(fileURLWithPath: path)
    let parent = url.deletingLastPathComponent()
    do {
        try FileManager.default.createDirectory(at: parent, withIntermediateDirectories: true)
    } catch {
        fail("cannot create parent directory for \(label): \(error)")
    }
    return url
}

func ensureDirectory(_ path: String?, label: String) -> URL {
    guard let path = path else { fail("missing --\(label)") }
    let url = URL(fileURLWithPath: path)
    do {
        try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
    } catch {
        fail("cannot create directory for \(label): \(error)")
    }
    return url
}

func buildConfiguration(options: Options) throws -> VZVirtualMachineConfiguration {
    let diskURL = requireFile(options.disk, label: "disk")
    let seedURL = requireFile(options.seed, label: "seed")
    let efiURL = ensureParentDirectory(options.efi, label: "efi")
    let serialURL = ensureParentDirectory(options.serial, label: "serial")
    let aptCacheURL = ensureDirectory(options.aptCache, label: "apt-cache")

    let configuration = VZVirtualMachineConfiguration()
    configuration.cpuCount = options.cpus
    configuration.memorySize = options.memoryMiB * 1024 * 1024

    let bootLoader = VZEFIBootLoader()
    if FileManager.default.fileExists(atPath: efiURL.path) {
        bootLoader.variableStore = VZEFIVariableStore(url: efiURL)
    } else {
        bootLoader.variableStore = try VZEFIVariableStore(creatingVariableStoreAt: efiURL)
    }
    configuration.bootLoader = bootLoader

    let diskAttachment = try VZDiskImageStorageDeviceAttachment(url: diskURL, readOnly: false)
    let seedAttachment = try VZDiskImageStorageDeviceAttachment(url: seedURL, readOnly: true)
    configuration.storageDevices = [
        VZVirtioBlockDeviceConfiguration(attachment: diskAttachment),
        VZVirtioBlockDeviceConfiguration(attachment: seedAttachment),
    ]

    let network = VZVirtioNetworkDeviceConfiguration()
    if let mac = options.mac {
        guard let address = VZMACAddress(string: mac) else { fail("invalid --mac \(mac)") }
        network.macAddress = address
    }
    network.attachment = VZNATNetworkDeviceAttachment()
    configuration.networkDevices = [network]

    configuration.entropyDevices = [VZVirtioEntropyDeviceConfiguration()]
    configuration.memoryBalloonDevices = [VZVirtioTraditionalMemoryBalloonDeviceConfiguration()]

    let sharedDirectory = VZSharedDirectory(url: aptCacheURL, readOnly: false)
    let directoryShare = VZSingleDirectoryShare(directory: sharedDirectory)
    let fileSystem = VZVirtioFileSystemDeviceConfiguration(tag: "agent_sandbox_apt_cache")
    fileSystem.share = directoryShare
    configuration.directorySharingDevices = [fileSystem]

    FileManager.default.createFile(atPath: serialURL.path, contents: nil)
    let serialHandle = try FileHandle(forWritingTo: serialURL)
    let serial = VZVirtioConsoleDeviceSerialPortConfiguration()
    serial.attachment = VZFileHandleSerialPortAttachment(fileHandleForReading: nil, fileHandleForWriting: serialHandle)
    configuration.serialPorts = [serial]

    try configuration.validate()
    return configuration
}

final class VMDelegate: NSObject, VZVirtualMachineDelegate {
    func guestDidStop(_ virtualMachine: VZVirtualMachine) {
        exit(0)
    }

    func virtualMachine(_ virtualMachine: VZVirtualMachine, didStopWithError error: Error) {
        fail("virtual machine stopped with error: \(error)")
    }
}

let options = parseOptions(Array(CommandLine.arguments.dropFirst()))

do {
    let configuration = try buildConfiguration(options: options)
    switch options.command {
    case "validate":
        print("valid")
    case "start":
        let virtualMachine = VZVirtualMachine(configuration: configuration)
        let delegate = VMDelegate()
        virtualMachine.delegate = delegate
        virtualMachine.start { result in
            switch result {
            case .success:
                print("started")
            case .failure(let error):
                fail("failed to start virtual machine: \(error)")
            }
        }
        RunLoop.main.run()
    default:
        fail("unknown command \(options.command)")
    }
} catch {
    fail(String(describing: error))
}
