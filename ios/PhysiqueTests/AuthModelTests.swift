import Foundation
import Testing

@testable import PhysiqueCore

/// 保存先を差し替えるための偽ストア。Keychain は macOS のテストで使えない
final class MemorySessionStore: SessionStore, @unchecked Sendable {
    var stored: Session?
    func load() -> Session? { stored }
    func save(_ s: Session) { stored = s }
    func clear() { stored = nil }
}

private let supabase = URL(string: "https://project.supabase.co")!

private func tokenJSON(_ access: String, expiresIn: Int = 3600) -> String {
    #"{"access_token":"\#(access)","refresh_token":"ref","expires_in":\#(expiresIn)}"#
}

@Suite("ログインの状態")
@MainActor
struct AuthModelTests {
    @Test("保存が無ければ未ログイン")
    func startsSignedOut() {
        let model = AuthModel(
            auth: AuthClient(url: supabase, anonKey: "k", transport: FakeTransport()),
            store: MemorySessionStore()
        )

        #expect(!model.isSignedIn)
    }

    @Test("保存があればログイン済みで始まる")
    func restores() {
        // **アプリを開くたびにログインさせない**
        let store = MemorySessionStore()
        store.stored = Session(
            accessToken: "saved", refreshToken: "r",
            expiresAt: Date().addingTimeInterval(3600)
        )
        let model = AuthModel(
            auth: AuthClient(url: supabase, anonKey: "k", transport: FakeTransport()),
            store: store
        )

        #expect(model.isSignedIn)
    }

    @Test("ログインするとセッションを保存する")
    func signInSaves() async {
        let store = MemorySessionStore()
        let t = FakeTransport(json: tokenJSON("jwt-1"))
        let model = AuthModel(auth: AuthClient(url: supabase, anonKey: "k", transport: t), store: store)

        await model.signIn(email: "a@b.c", password: "pw")

        #expect(model.isSignedIn)
        #expect(store.stored?.accessToken == "jwt-1")
        #expect(model.errorMessage == nil)
    }

    @Test("失敗したら理由を出し、ログイン状態にしない")
    func signInFails() async {
        let store = MemorySessionStore()
        let t = FakeTransport(json: #"{"error":"invalid_grant"}"#, status: 400)
        let model = AuthModel(auth: AuthClient(url: supabase, anonKey: "k", transport: t), store: store)

        await model.signIn(email: "a@b.c", password: "wrong")

        #expect(!model.isSignedIn)
        #expect(model.errorMessage == "メールアドレスかパスワードが違う")
        #expect(store.stored == nil)
    }

    @Test("ログアウトすると保存を消す")
    func signOutClears() async {
        let store = MemorySessionStore()
        store.stored = Session(accessToken: "a", refreshToken: "r", expiresAt: Date().addingTimeInterval(3600))
        let model = AuthModel(auth: AuthClient(url: supabase, anonKey: "k", transport: FakeTransport()), store: store)

        await model.signOut()

        #expect(!model.isSignedIn)
        #expect(store.stored == nil)
    }

    @Test("期限内ならそのトークンを返す")
    func tokenWhenFresh() async throws {
        let store = MemorySessionStore()
        store.stored = Session(accessToken: "fresh", refreshToken: "r", expiresAt: Date().addingTimeInterval(3600))
        let t = FakeTransport(json: tokenJSON("should-not-be-used"))
        let model = AuthModel(auth: AuthClient(url: supabase, anonKey: "k", transport: t), store: store)

        #expect(try await model.accessToken() == "fresh")
        // 取り直していないこと
        #expect(t.requests.isEmpty)
    }

    @Test("**期限が切れていれば取り直す**")
    func refreshesWhenExpired() async throws {
        let store = MemorySessionStore()
        store.stored = Session(accessToken: "old", refreshToken: "r", expiresAt: Date().addingTimeInterval(-1))
        let t = FakeTransport(json: tokenJSON("renewed"))
        let model = AuthModel(auth: AuthClient(url: supabase, anonKey: "k", transport: t), store: store)

        #expect(try await model.accessToken() == "renewed")
        #expect(store.stored?.accessToken == "renewed")
    }

    @Test("取り直しが拒否されたらログアウトする")
    func signsOutWhenRefreshRejected() async {
        // 保存が残っていると、毎回 401 になる画面を延々と見せることになる
        let store = MemorySessionStore()
        store.stored = Session(accessToken: "old", refreshToken: "dead", expiresAt: Date().addingTimeInterval(-1))
        let t = FakeTransport(json: #"{"error":"invalid_grant"}"#, status: 400)
        let model = AuthModel(auth: AuthClient(url: supabase, anonKey: "k", transport: t), store: store)

        _ = try? await model.accessToken()

        #expect(!model.isSignedIn)
        #expect(store.stored == nil)
    }
}
