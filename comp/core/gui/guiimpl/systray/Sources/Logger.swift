// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

import Foundation
import os.log

enum LogLevel {
    case debug, info, error
}

class Logger {
    private static var useFileLogging = false
    private static let osLog = OSLog(subsystem: "com.datadoghq.agent", category: "GUI")

    /// Configure logging mode at startup
    /// - Parameter useFileLogging: If true, uses NSLog (file-based); if false, uses os_log (unified logging)
    static func configure(useFileLogging: Bool) {
        self.useFileLogging = useFileLogging
        if useFileLogging {
            NSLog("[Logger] File-based logging enabled")
        } else {
            os_log("[Logger] Unified logging (os_log) enabled", log: osLog, type: .info)
        }
    }

    /// Log a message with specified level and context
    /// - Parameters:
    ///   - message: The message to log
    ///   - level: Log level (debug, info, error)
    ///   - context: Optional context string (e.g., class name)
    static func log(_ message: String, level: LogLevel = .info, context: String = "") {
        let prefix = context.isEmpty ? "" : "[\(context)] "
        let fullMessage = "\(prefix)\(message)"

        if useFileLogging {
            // Use NSLog which outputs to stdout/stderr (redirected to file by LaunchAgent)
            NSLog(fullMessage)
        } else {
            // Use os_log (default) - automatic log size management by macOS
            switch level {
            case .debug:
                os_log("%{public}@", log: osLog, type: .debug, fullMessage)
            case .info:
                os_log("%{public}@", log: osLog, type: .info, fullMessage)
            case .error:
                os_log("%{public}@", log: osLog, type: .error, fullMessage)
            }
        }
    }

    /// Log a debug message
    static func debug(_ message: String, context: String = "") {
        log(message, level: .debug, context: context)
    }

    /// Log an info message
    static func info(_ message: String, context: String = "") {
        log(message, level: .info, context: context)
    }

    /// Log an error message
    static func error(_ message: String, context: String = "") {
        log(message, level: .error, context: context)
    }
}
