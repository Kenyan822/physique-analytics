import Foundation

/// ログイン中のセッション。
struct Session: Sendable, Codable, Equatable {
    let accessToken: String
    let refreshToken: String
    /// いつ切れるか。**秒数ではなく時刻で持つ** —— 秒数のままだと
    /// 持ち回るたびに「いつ取得したか」と突き合わせることになる
    let expiresAt: Date

    /// まだ使えるか。
    ///
    /// **期限の直前は使えない扱いにする。** 送信中に切れると 401 になるので、
    /// 余裕を持って先に取り直す。
    func isUsable(at now: Date = Date()) -> Bool {
        expiresAt.timeIntervalSince(now) > Self.margin
    }

    /// 期限までの余裕。往復と時計のずれを見込む
    static let margin: TimeInterval = 60
}

enum AuthError: Error, Equatable, LocalizedError {
    /// メールかパスワードが違う。**どちらかは区別しない**
    case invalidCredentials
    /// セッションが切れた。ログインし直しが要る
    case needsSignIn
    case server(status: Int)
    case transport(String)

    var errorDescription: String? {
        switch self {
        case .invalidCredentials: return "メールアドレスかパスワードが違う"
        case .needsSignIn: return "ログインし直してください"
        case let .server(status): return "ログインできない（HTTP \(status)）"
        case let .transport(m): return "通信できない: \(m)"
        }
    }
}

/// Supabase Auth（GoTrue）を直接叩く。
///
/// **SDK を入れていない。** 使うのはログイン・取り直し・ログアウトの3つだけで、
/// アプリは既に `HTTPTransport` を持っている。依存を1つ増やすより、
/// 既存の層に載せる方が差し替えもテストも効く。
struct AuthClient: Sendable {
    let url: URL
    /// 公開前提の鍵。単体では何も読めない（DB は RLS で閉じている）
    let anonKey: String
    var transport: HTTPTransport

    init(url: URL, anonKey: String, transport: HTTPTransport = URLSessionTransport()) {
        self.url = url
        self.anonKey = anonKey
        self.transport = transport
    }

    func signIn(email: String, password: String, now: Date = Date()) async throws -> Session {
        try await token(
            grant: "password",
            body: ["email": email, "password": password],
            now: now
        )
    }

    func refresh(refreshToken: String, now: Date = Date()) async throws -> Session {
        try await token(
            grant: "refresh_token",
            body: ["refresh_token": refreshToken],
            now: now
        )
    }

    /// ログアウト。**失敗しても握りつぶす。**
    /// 手元のセッションを捨てる方が本質で、サーバ側の失効は届けば良い
    func signOut(accessToken: String) async {
        var req = URLRequest(url: url.appending(path: "auth/v1/logout"))
        req.httpMethod = "POST"
        req.setValue(anonKey, forHTTPHeaderField: "apikey")
        req.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")

        _ = try? await transport.send(req)
    }

    private func token(grant: String, body: [String: String], now: Date) async throws -> Session {
        var components = URLComponents(
            url: url.appending(path: "auth/v1/token"), resolvingAgainstBaseURL: false
        )!
        components.queryItems = [URLQueryItem(name: "grant_type", value: grant)]

        var req = URLRequest(url: components.url!)
        req.httpMethod = "POST"
        // **apikey を付けないと Supabase が受け付けない**
        req.setValue(anonKey, forHTTPHeaderField: "apikey")
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try JSONEncoder().encode(body)

        let (data, http): (Data, HTTPURLResponse)
        do {
            (data, http) = try await transport.send(req)
        } catch {
            throw AuthError.transport(error.localizedDescription)
        }

        guard http.statusCode == 200 else {
            // 400 は資格情報の誤りか、リフレッシュトークンの失効
            if http.statusCode == 400 {
                throw grant == "password" ? AuthError.invalidCredentials : AuthError.needsSignIn
            }
            if http.statusCode == 401 { throw AuthError.needsSignIn }
            throw AuthError.server(status: http.statusCode)
        }

        let res = try JSONDecoder().decode(TokenResponse.self, from: data)

        return Session(
            accessToken: res.access_token,
            refreshToken: res.refresh_token,
            expiresAt: now.addingTimeInterval(TimeInterval(res.expires_in))
        )
    }
}

// swiftlint:disable identifier_name
/// Supabase の応答。スネークケースのまま受ける
private struct TokenResponse: Decodable {
    let access_token: String
    let refresh_token: String
    let expires_in: Int
}
// swiftlint:enable identifier_name
