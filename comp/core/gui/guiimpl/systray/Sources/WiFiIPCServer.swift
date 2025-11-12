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

/// WiFiIPCServer handles Unix socket communication for WiFi data
class WiFiIPCServer {
    private let wifiDataProvider: WiFiDataProvider
    private var socketFileDescriptor: Int32 = -1
    private var isRunning = false
    private var acceptQueue: DispatchQueue
    private let socketPath: String

    init(wifiDataProvider: WiFiDataProvider) {
        self.wifiDataProvider = wifiDataProvider

        // Create socket path based on current user's UID
        let uid = getuid()
        self.socketPath = "/var/run/datadog-agent/wifi-\(uid).sock"
        // Use .background QoS for agent telemetry collection (not user-facing work)
        self.acceptQueue = DispatchQueue(label: "com.datadoghq.wifi.ipc", qos: .background)

        NSLog("[WiFiIPCServer] Initialized with socket path: \(socketPath)")
    }

    /// Start the IPC server
    func start() throws {
        guard !isRunning else {
            NSLog("[WiFiIPCServer] Server already running")
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

        // Set umask to create socket with 0o660 permissions atomically
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
            NSLog("[WiFiIPCServer] Socket created with permissions: \(String(format: "%o", posixPerms.uint16Value))")
        }

        // Listen for connections
        guard listen(socketFileDescriptor, 5) >= 0 else {
            close(socketFileDescriptor)
            try? FileManager.default.removeItem(atPath: socketPath)
            throw NSError(domain: "WiFiIPCServer", code: 4,
                         userInfo: [NSLocalizedDescriptionKey: "Failed to listen on socket: \(String(cString: strerror(errno)))"])
        }

        isRunning = true
        NSLog("[WiFiIPCServer] Server started on \(socketPath)")

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

        NSLog("[WiFiIPCServer] Server stopped")
    }

    // Private Methods
    private func createSocketDirectory() throws {
        let socketDir = "/var/run/datadog-agent"
        let fileManager = FileManager.default

        // Check if directory exists
        var isDirectory: ObjCBool = false
        if fileManager.fileExists(atPath: socketDir, isDirectory: &isDirectory) {
            if isDirectory.boolValue {
                NSLog("[WiFiIPCServer] Socket directory exists: \(socketDir)")
                return // Directory already exists
            }
            // Path exists but is not a directory
            throw NSError(domain: "WiFiIPCServer", code: 5,
                         userInfo: [NSLocalizedDescriptionKey: "\(socketDir) exists but is not a directory"])
        }

        // Try to create directory (may fail if we don't have permissions)
        do {
            try fileManager.createDirectory(atPath: socketDir, withIntermediateDirectories: true, attributes: nil)
            NSLog("[WiFiIPCServer] Created socket directory: \(socketDir)")
        } catch {
            // Directory creation failed - log detailed error
            NSLog("[WiFiIPCServer] ERROR: Cannot create socket directory \(socketDir)")
            NSLog("[WiFiIPCServer] ERROR: \(error.localizedDescription)")
            NSLog("[WiFiIPCServer] The agent installer must create this directory during installation")
            throw NSError(domain: "WiFiIPCServer", code: 6,
                         userInfo: [NSLocalizedDescriptionKey: "Cannot create socket directory \(socketDir). Ensure /var/run/datadog-agent exists with proper permissions. Error: \(error.localizedDescription)"])
        }
    }

    private func acceptLoop() {
        NSLog("[WiFiIPCServer] Accept loop started")

        while isRunning {
            let clientFD = accept(socketFileDescriptor, nil, nil)

            if clientFD < 0 {
                if isRunning {
                    NSLog("[WiFiIPCServer] Accept failed: \(String(cString: strerror(errno)))")
                }
                continue
            }

            // Handle client connection on a separate dispatch (background QoS for telemetry)
            DispatchQueue.global(qos: .background).async { [weak self] in
                self?.handleClient(clientFD)
            }
        }

        NSLog("[WiFiIPCServer] Accept loop ended")
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
            NSLog("[WiFiIPCServer] Failed to read from client")
            return
        }

        let requestData = Data(buffer[0..<bytesRead])

        // Parse request
        guard let request = try? JSONDecoder().decode(WiFiIPCRequest.self, from: requestData) else {
            NSLog("[WiFiIPCServer] Failed to decode request")
            sendErrorResponse(clientFD, error: "Invalid request format")
            return
        }

        NSLog("[WiFiIPCServer] Received command: \(request.command)")

        // Handle command
        switch request.command {
        case "get_wifi_info":
            let wifiData = wifiDataProvider.getWiFiInfo()
            let response = WiFiIPCResponse(success: true, data: wifiData, error: nil)
            sendResponse(clientFD, response: response)

        default:
            NSLog("[WiFiIPCServer] Unknown command: \(request.command)")
            sendErrorResponse(clientFD, error: "Unknown command: \(request.command)")
        }
    }

    private func sendResponse(_ clientFD: Int32, response: WiFiIPCResponse) {
        guard let responseData = try? JSONEncoder().encode(response) else {
            NSLog("[WiFiIPCServer] Failed to encode response")
            return
        }

        var data = responseData
        data.append(contentsOf: [UInt8(ascii: "\n")]) // Add newline delimiter
        
        data.withUnsafeBytes { ptr in
            let bytesPtr = ptr.baseAddress?.assumingMemoryBound(to: UInt8.self)
            _ = write(clientFD, bytesPtr, data.count)
        }
    }

    private func sendErrorResponse(_ clientFD: Int32, error: String) {
        let response = WiFiIPCResponse(success: false, data: nil, error: error)
        sendResponse(clientFD, response: response)
    }

    deinit {
        stop()
    }
}
