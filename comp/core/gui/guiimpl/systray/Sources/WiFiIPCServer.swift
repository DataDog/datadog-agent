import Foundation

/// Request structure for IPC commands
struct WiFiIPCRequest: Codable {
    let command: String
}

/// Response structure for IPC
struct WiFiIPCResponse: Codable {
    let success: Bool
    let data: WiFiData?
    let error: String?
}

/// Response structure for desktop ready IPC
struct DesktopReadyIPCResponse: Codable {
    let success: Bool
    let data: DesktopReadyTracker.DesktopReadyData?
    let error: String?
}

/// WiFiIPCServer handles Unix socket communication for WiFi data
class WiFiIPCServer {
    private let wifiDataProvider: WiFiDataProvider
    private weak var desktopReadyTracker: DesktopReadyTracker?
    private var socketFileDescriptor: Int32 = -1
    private var isRunning = false
    private var acceptQueue: DispatchQueue
    private let socketPath: String

    init(wifiDataProvider: WiFiDataProvider, desktopReadyTracker: DesktopReadyTracker?) {
        self.wifiDataProvider = wifiDataProvider
        self.desktopReadyTracker = desktopReadyTracker

        // Get installation path from environment (set by LaunchAgent) or use default
        let installPath = ProcessInfo.processInfo.environment["DD_INSTALL_PATH"] ?? "/opt/datadog-agent"

        // Create socket path based on current user's UID
        let uid = getuid()
        self.socketPath = "\(installPath)/run/ipc/gui-\(uid).sock"
        // Use .background QoS for agent telemetry collection (not user-facing work)
        self.acceptQueue = DispatchQueue(label: "com.datadoghq.wifi.ipc", qos: .background)

        Logger.info("Initialized with socket path: \(socketPath)", context: "WiFiIPCServer")
    }

    /// Start the IPC server
    func start() throws {
        guard !isRunning else {
            Logger.info("Server already running", context: "WiFiIPCServer")
            return
        }

        // Create socket directory if it doesn't exist
        try createSocketDirectory()

        // Remove existing socket file if present
        try? FileManager.default.removeItem(atPath: socketPath)

        // Create Unix domain socket
        socketFileDescriptor = socket(AF_UNIX, SOCK_STREAM, 0)
        guard socketFileDescriptor >= 0 else {
            throw NSError(domain: "WiFiIPCServer", code: 1, 
                         userInfo: [NSLocalizedDescriptionKey: "Failed to create socket: \(String(cString: strerror(errno)))"])
        }

        // Set socket to non-blocking and reusable
        var value: Int32 = 1
        setsockopt(socketFileDescriptor, SOL_SOCKET, SO_REUSEADDR, &value, socklen_t(MemoryLayout<Int32>.size))

        // Bind socket to path
        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)

        let pathBytes = socketPath.utf8CString
        guard pathBytes.count <= MemoryLayout.size(ofValue: addr.sun_path) else {
            close(socketFileDescriptor)
            throw NSError(domain: "WiFiIPCServer", code: 2,
                         userInfo: [NSLocalizedDescriptionKey: "Socket path too long"])
        }

        withUnsafeMutableBytes(of: &addr.sun_path) { ptr in
            pathBytes.withUnsafeBytes { pathPtr in
                ptr.copyMemory(from: pathPtr)
            }
        }

        // Set umask to create socket with 0o660 (-rw-rw----) permissions atomically
        let oldUmask = umask(0o117)
        defer {
            // Restore original umask after bind
            umask(oldUmask)
        }

        let bindResult = withUnsafePointer(to: &addr) { ptr in
            ptr.withMemoryRebound(to: sockaddr.self, capacity: 1) { sockaddrPtr in
                Darwin.bind(socketFileDescriptor, sockaddrPtr, socklen_t(MemoryLayout<sockaddr_un>.size))
            }
        }

        guard bindResult >= 0 else {
            close(socketFileDescriptor)
            throw NSError(domain: "WiFiIPCServer", code: 3,
                         userInfo: [NSLocalizedDescriptionKey: "Failed to bind socket: \(String(cString: strerror(errno)))"])
        }

        // Verify permissions (optional logging)
        if let attrs = try? FileManager.default.attributesOfItem(atPath: socketPath),
           let posixPerms = attrs[.posixPermissions] as? NSNumber {
            Logger.debug("Socket created with permissions: \(String(format: "%o", posixPerms.uint16Value))", context: "WiFiIPCServer")
        }

        // Listen for connections
        guard listen(socketFileDescriptor, 5) >= 0 else {
            close(socketFileDescriptor)
            try? FileManager.default.removeItem(atPath: socketPath)
            throw NSError(domain: "WiFiIPCServer", code: 4,
                         userInfo: [NSLocalizedDescriptionKey: "Failed to listen on socket: \(String(cString: strerror(errno)))"])
        }

        isRunning = true
        Logger.info("Server started on \(socketPath)", context: "WiFiIPCServer")

        // Start accepting connections on background queue
        acceptQueue.async { [weak self] in
            self?.acceptLoop()
        }
    }

    /// Stop the IPC server
    func stop() {
        guard isRunning else { return }

        isRunning = false

        if socketFileDescriptor >= 0 {
            close(socketFileDescriptor)
            socketFileDescriptor = -1
        }

        // Clean up socket file
        try? FileManager.default.removeItem(atPath: socketPath)

        Logger.info("Server stopped", context: "WiFiIPCServer")
    }

    // Private Methods
    private func createSocketDirectory() throws {
        // Get installation path from environment (set by LaunchAgent) or use default
        let installPath = ProcessInfo.processInfo.environment["DD_INSTALL_PATH"] ?? "/opt/datadog-agent"
        let socketDir = "\(installPath)/run/ipc"
        let fileManager = FileManager.default

        // Check if directory exists
        var isDirectory: ObjCBool = false
        if fileManager.fileExists(atPath: socketDir, isDirectory: &isDirectory) {
            if isDirectory.boolValue {
                Logger.debug("Socket directory exists: \(socketDir)", context: "WiFiIPCServer")
                return // Directory already exists
            }
            // Path exists but is not a directory
            throw NSError(domain: "WiFiIPCServer", code: 5,
                         userInfo: [NSLocalizedDescriptionKey: "\(socketDir) exists but is not a directory"])
        }

        // Directory should have been created by the installer
        Logger.error("Socket directory does not exist: \(socketDir)", context: "WiFiIPCServer")
        Logger.error("The agent installer should create this directory during installation", context: "WiFiIPCServer")
        throw NSError(domain: "WiFiIPCServer", code: 6,
                     userInfo: [NSLocalizedDescriptionKey: "Socket directory \(socketDir) does not exist. The directory should be created during agent installation."])
    }

    private func acceptLoop() {
        Logger.info("Accept loop started", context: "WiFiIPCServer")

        while isRunning {
            let clientFD = accept(socketFileDescriptor, nil, nil)

            if clientFD < 0 {
                if isRunning {
                    Logger.error("Accept failed: \(String(cString: strerror(errno)))", context: "WiFiIPCServer")
                }
                continue
            }

            // Handle client connection on a separate dispatch (background QoS for telemetry)
            DispatchQueue.global(qos: .background).async { [weak self] in
                self?.handleClient(clientFD)
            }
        }

        Logger.info("Accept loop ended", context: "WiFiIPCServer")
    }

    private func handleClient(_ clientFD: Int32) {
        defer {
            close(clientFD)
        }

        // Set read timeout
        var timeout = timeval(tv_sec: 5, tv_usec: 0)
        setsockopt(clientFD, SOL_SOCKET, SO_RCVTIMEO, &timeout, socklen_t(MemoryLayout<timeval>.size))

        // Read request data
        var buffer = [UInt8](repeating: 0, count: 4096)
        let bytesRead = read(clientFD, &buffer, buffer.count)

        guard bytesRead > 0 else {
            Logger.error("Failed to read from client", context: "WiFiIPCServer")
            return
        }

        let requestData = Data(buffer[0..<bytesRead])

        // Parse request
        guard let request = try? JSONDecoder().decode(WiFiIPCRequest.self, from: requestData) else {
            Logger.error("Failed to decode request", context: "WiFiIPCServer")
            sendErrorResponse(clientFD, error: "Invalid request format")
            return
        }

        // Handle command
        switch request.command {
        case "get_wifi_info":
            let wifiData = wifiDataProvider.getWiFiInfo()
            let response = WiFiIPCResponse(success: true, data: wifiData, error: nil)
            sendResponse(clientFD, response: response)

        case "get_desktop_ready":
            if let tracker = desktopReadyTracker {
                let data = tracker.getDesktopReadyData()
                let response = DesktopReadyIPCResponse(success: true, data: data, error: nil)
                sendDesktopReadyResponse(clientFD, response: response)
            } else {
                let response = DesktopReadyIPCResponse(success: false, data: nil, error: "Desktop ready tracker not initialized")
                sendDesktopReadyResponse(clientFD, response: response)
            }

        default:
            Logger.error("Unknown command: \(request.command)", context: "WiFiIPCServer")
            sendErrorResponse(clientFD, error: "Unknown command: \(request.command)")
        }
    }

    private func sendResponse(_ clientFD: Int32, response: WiFiIPCResponse) {
        guard let responseData = try? JSONEncoder().encode(response) else {
            Logger.error("Failed to encode response", context: "WiFiIPCServer")
            return
        }

        var data = responseData
        data.append(contentsOf: [UInt8(ascii: "\n")]) // Add newline delimiter
        
        data.withUnsafeBytes { ptr in
            let bytesPtr = ptr.baseAddress?.assumingMemoryBound(to: UInt8.self)
            var bytesWritten = write(clientFD, bytesPtr, data.count)

            // Handle write errors gracefully (client may disconnect before response is fully sent)
            // With SIGPIPE ignored in main.swift, write() returns -1 instead of crashing the app
            if bytesWritten < 0 {
                if errno == EPIPE {
                    // Client disconnected before response fully sent - this is normal for short-lived connections
                    Logger.debug("Client disconnected before response sent (EPIPE)", context: "WiFiIPCServer")
                } else {
                    Logger.error("Write failed: \(String(cString: strerror(errno)))", context: "WiFiIPCServer")
                }
            } else if bytesWritten < data.count {
                // Partial write - rare but possible if socket buffer is full
                Logger.info("Partial write: \(bytesWritten)/\(data.count) bytes sent, retrying...", context: "WiFiIPCServer")

                // Retry once for remaining bytes
                guard let validBytesPtr = bytesPtr else {
                    Logger.error("Cannot retry: invalid buffer pointer", context: "WiFiIPCServer")
                    return
                }

                let remaining = data.count - bytesWritten
                let remainingPtr = validBytesPtr.advanced(by: bytesWritten)
                let retryWritten = write(clientFD, remainingPtr, remaining)

                if retryWritten < 0 {
                    if errno == EPIPE {
                        Logger.debug("Client disconnected during retry (EPIPE)", context: "WiFiIPCServer")
                    } else {
                        Logger.error("Retry write failed: \(String(cString: strerror(errno)))", context: "WiFiIPCServer")
                    }
                } else if retryWritten < remaining {
                    // Still partial after retry - log error (rare)
                    Logger.error("Partial write after retry: \(bytesWritten + retryWritten)/\(data.count) bytes sent", context: "WiFiIPCServer")
                } else {
                    // Retry succeeded
                    Logger.debug("Retry write completed successfully", context: "WiFiIPCServer")
                }
            }
            // Success case (bytesWritten == data.count) - no logging needed for normal operation
        }
    }

    private func sendErrorResponse(_ clientFD: Int32, error: String) {
        let response = WiFiIPCResponse(success: false, data: nil, error: error)
        sendResponse(clientFD, response: response)
    }

    private func sendDesktopReadyResponse(_ clientFD: Int32, response: DesktopReadyIPCResponse) {
        guard let responseData = try? JSONEncoder().encode(response) else {
            Logger.error("Failed to encode desktop ready response", context: "WiFiIPCServer")
            return
        }

        var data = responseData
        data.append(contentsOf: [UInt8(ascii: "\n")]) // Add newline delimiter

        data.withUnsafeBytes { ptr in
            guard let bytesPtr = ptr.baseAddress?.assumingMemoryBound(to: UInt8.self) else { return }
            let bytesWritten = write(clientFD, bytesPtr, data.count)
            if bytesWritten < 0 && errno != EPIPE {
                Logger.error("Write failed: \(String(cString: strerror(errno)))", context: "WiFiIPCServer")
            }
        }
    }

    deinit {
        stop()
    }
}
