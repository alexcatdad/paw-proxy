import Foundation

public enum DaemonClientError: Error, Equatable, CustomStringConvertible {
    case socketPathTooLong
    case connect(Int32)
    case io(Int32)
    case timeout
    case responseTooLarge
    case malformedResponse
    case httpStatus(Int)

    public var description: String {
        switch self {
        case .socketPathTooLong: return "socket path too long"
        case .connect(let e): return "cannot connect to the daemon (\(String(cString: strerror(e))))"
        case .io(let e): return "socket error (\(String(cString: strerror(e))))"
        case .timeout: return "the daemon did not answer in time"
        case .responseTooLarge: return "daemon response too large"
        case .malformedResponse: return "malformed daemon response"
        case .httpStatus(let s): return "daemon answered HTTP \(s)"
        }
    }
}

/// Minimal HTTP client for the daemon's Unix-socket API.
///
/// It sends HTTP/1.0 requests so Go's server delimits the body by closing
/// the connection (no chunked encoding), which keeps parsing trivial.
public struct DaemonClient: Sendable {
    public var socketPath: String
    public var timeout: TimeInterval
    public var maxResponseBytes: Int

    public init(socketPath: String, timeout: TimeInterval = 5, maxResponseBytes: Int = 1 << 20) {
        self.socketPath = socketPath
        self.timeout = timeout
        self.maxResponseBytes = maxResponseBytes
    }

    public func health() throws -> Health {
        try DaemonJSON.decoder().decode(Health.self, from: get("/health"))
    }

    public func routes() throws -> [Route] {
        try DaemonJSON.decoder().decode([Route].self, from: get("/routes"))
    }

    /// Performs GET path and returns the body of a 2xx response.
    public func get(_ path: String) throws -> Data {
        let fd = socket(AF_UNIX, SOCK_STREAM, 0)
        guard fd >= 0 else { throw DaemonClientError.io(errno) }
        defer { close(fd) }

        var tv = timeval(tv_sec: Int(timeout), tv_usec: Int32((timeout - floor(timeout)) * 1_000_000))
        setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, socklen_t(MemoryLayout<timeval>.size))
        setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &tv, socklen_t(MemoryLayout<timeval>.size))
        var nosigpipe: Int32 = 1
        setsockopt(fd, SOL_SOCKET, SO_NOSIGPIPE, &nosigpipe, socklen_t(MemoryLayout<Int32>.size))

        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        let pathBytes = Array(socketPath.utf8)
        let capacity = MemoryLayout.size(ofValue: addr.sun_path)
        guard pathBytes.count < capacity else { throw DaemonClientError.socketPathTooLong }
        withUnsafeMutableBytes(of: &addr.sun_path) { raw in
            raw.copyBytes(from: pathBytes)
            raw[pathBytes.count] = 0
        }
        let len = socklen_t(MemoryLayout<sockaddr_un>.size)
        let rc = withUnsafePointer(to: &addr) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) { connect(fd, $0, len) }
        }
        guard rc == 0 else { throw DaemonClientError.connect(errno) }

        let request = Array("GET \(path) HTTP/1.0\r\nHost: unix\r\nAccept: application/json\r\n\r\n".utf8)
        var sent = 0
        while sent < request.count {
            let n = request[sent...].withUnsafeBytes { write(fd, $0.baseAddress, $0.count) }
            if n < 0 { throw errno == EAGAIN ? DaemonClientError.timeout : DaemonClientError.io(errno) }
            sent += n
        }

        var response = Data()
        var buf = [UInt8](repeating: 0, count: 16 * 1024)
        while true {
            let n = read(fd, &buf, buf.count)
            if n == 0 { break }
            if n < 0 {
                if errno == EINTR { continue }
                throw errno == EAGAIN ? DaemonClientError.timeout : DaemonClientError.io(errno)
            }
            response.append(contentsOf: buf[0..<n])
            if response.count > maxResponseBytes { throw DaemonClientError.responseTooLarge }
        }
        return try Self.parse(response)
    }

    /// Splits a raw HTTP/1.x response and returns the body of a 2xx reply.
    static func parse(_ response: Data) throws -> Data {
        let separator = Data("\r\n\r\n".utf8)
        guard let headerEnd = response.range(of: separator) else { throw DaemonClientError.malformedResponse }
        let head = String(decoding: response[..<headerEnd.lowerBound], as: UTF8.self)
        let statusLine = head.split(separator: "\r\n", maxSplits: 1).first ?? ""
        let parts = statusLine.split(separator: " ")
        guard parts.count >= 2, parts[0].hasPrefix("HTTP/1."), let status = Int(parts[1]) else {
            throw DaemonClientError.malformedResponse
        }
        guard (200..<300).contains(status) else { throw DaemonClientError.httpStatus(status) }
        return response[headerEnd.upperBound...]
    }
}
