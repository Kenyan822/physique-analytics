import Foundation
import Testing

@testable import PhysiqueCore

private let supabase = URL(string: "https://project.supabase.co")!

@Suite("Supabase の認証")
struct AuthClientTests {
    @Test("メールとパスワードでトークンを取る")
    func signIn() async throws {
        let t = FakeTransport(json: """
        {"access_token":"jwt-abc","refresh_token":"ref-xyz","expires_in":3600}
        """)
        let auth = AuthClient(url: supabase, anonKey: "anon-key", transport: t)

        let session = try await auth.signIn(email: "me@example.com", password: "pw")

        #expect(session.accessToken == "jwt-abc")
        #expect(session.refreshToken == "ref-xyz")

        let req = try #require(t.requests.first)
        #expect(req.url?.path == "/auth/v1/token")
        #expect(req.url?.query?.contains("grant_type=password") == true)
        // anon key はヘッダで送る。付けないと Supabase が受け付けない
        #expect(req.value(forHTTPHeaderField: "apikey") == "anon-key")
    }

    @Test("有効期限を時刻として持つ")
    func expiry() async throws {
        let t = FakeTransport(json: """
        {"access_token":"a","refresh_token":"r","expires_in":3600}
        """)
        let auth = AuthClient(url: supabase, anonKey: "k", transport: t)

        let now = Date(timeIntervalSince1970: 1_000_000)
        let session = try await auth.signIn(email: "a@b.c", password: "p", now: now)

        // 秒数のままだと「いつ切れるか」を持ち回るたびに計算し直すことになる
        #expect(session.expiresAt == now.addingTimeInterval(3600))
    }

    @Test("パスワードが違えば理由を区別しない")
    func wrongPassword() async throws {
        let t = FakeTransport(json: #"{"error":"invalid_grant","error_description":"Invalid login credentials"}"#, status: 400)
        let auth = AuthClient(url: supabase, anonKey: "k", transport: t)

        await #expect(throws: AuthError.invalidCredentials) {
            // 「メールが無い」と「パスワードが違う」を分けると、登録済みのメールを探れる
            _ = try await auth.signIn(email: "a@b.c", password: "wrong")
        }
    }

    @Test("リフレッシュトークンで取り直す")
    func refresh() async throws {
        let t = FakeTransport(json: """
        {"access_token":"jwt-new","refresh_token":"ref-new","expires_in":3600}
        """)
        let auth = AuthClient(url: supabase, anonKey: "k", transport: t)

        let session = try await auth.refresh(refreshToken: "ref-old")

        #expect(session.accessToken == "jwt-new")
        let req = try #require(t.requests.first)
        #expect(req.url?.query?.contains("grant_type=refresh_token") == true)
    }

    @Test("リフレッシュが拒否されたら再ログインが要ると分かる")
    func refreshRejected() async throws {
        let t = FakeTransport(json: #"{"error":"invalid_grant"}"#, status: 400)
        let auth = AuthClient(url: supabase, anonKey: "k", transport: t)

        await #expect(throws: AuthError.needsSignIn) {
            _ = try await auth.refresh(refreshToken: "expired")
        }
    }
}

@Suite("セッションの有効性")
struct SessionTests {
    private func session(expiresAt: Date) -> Session {
        Session(accessToken: "a", refreshToken: "r", expiresAt: expiresAt)
    }

    @Test("期限より前なら使える")
    func valid() {
        let now = Date()
        #expect(session(expiresAt: now.addingTimeInterval(600)).isUsable(at: now))
    }

    @Test("期限を過ぎていれば使えない")
    func expired() {
        let now = Date()
        #expect(!session(expiresAt: now.addingTimeInterval(-1)).isUsable(at: now))
    }

    @Test("**期限の直前も使えないことにする**")
    func nearExpiry() {
        // 送信中に切れると 401 になる。余裕を持って先に取り直す
        let now = Date()
        #expect(!session(expiresAt: now.addingTimeInterval(30)).isUsable(at: now))
    }
}
