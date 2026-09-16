import Foundation
import Observation

/// ログインの状態を持つ。
///
/// **アクセストークンはここからしか取らない。** 各画面が自分で期限を見ると、
/// 取り直しの処理が散らばって必ずどこかが漏れる。
@Observable
@MainActor
final class AuthModel {
    private(set) var session: Session?
    private(set) var errorMessage: String?
    private(set) var isWorking = false

    private let auth: AuthClient
    private let store: SessionStore

    var isSignedIn: Bool { session != nil }

    init(auth: AuthClient, store: SessionStore) {
        self.auth = auth
        self.store = store
        // **保存から復元する。** アプリを開くたびにログインさせない
        self.session = store.load()
    }

    func signIn(email: String, password: String) async {
        errorMessage = nil
        isWorking = true
        defer { isWorking = false }

        do {
            let s = try await auth.signIn(email: email, password: password)
            store.save(s)
            session = s
        } catch {
            errorMessage = (error as? LocalizedError)?.errorDescription ?? "ログインできない"
        }
    }

    func signOut() async {
        if let s = session { await auth.signOut(accessToken: s.accessToken) }
        store.clear()
        session = nil
    }

    /// API に付けるトークン。期限が近ければ取り直す。
    ///
    /// 取り直しを拒否されたらログアウトする。保存が残っていると、
    /// 毎回 401 になる画面を延々と見せることになる。
    func accessToken() async throws -> String {
        guard let current = session else { throw AuthError.needsSignIn }
        if current.isUsable() { return current.accessToken }

        do {
            let renewed = try await auth.refresh(refreshToken: current.refreshToken)
            store.save(renewed)
            session = renewed

            return renewed.accessToken
        } catch {
            store.clear()
            session = nil
            throw AuthError.needsSignIn
        }
    }
}
