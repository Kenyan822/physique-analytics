import Foundation

/// セッションの保存先。テストで差し替えるため。
protocol SessionStore: Sendable {
    func load() -> Session?
    func save(_ session: Session)
    func clear()
}

/// Keychain に保存する。
///
/// **UserDefaults に置かない。** リフレッシュトークンは、持っている限り
/// アクセストークンを取り直せる。端末のバックアップから平文で読める場所に
/// 置くものではない。
///
/// `kSecAttrAccessibleAfterFirstUnlock` にしているのは、再起動後に
/// ロックを解いていなくてもバックグラウンドの同期が動けるようにするため。
struct KeychainSessionStore: SessionStore {
    let service: String

    init(service: String = "com.physique.session") {
        self.service = service
    }

    private var query: [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: "supabase",
        ]
    }

    func load() -> Session? {
        var q = query
        q[kSecReturnData as String] = true
        q[kSecMatchLimit as String] = kSecMatchLimitOne

        var item: CFTypeRef?
        guard SecItemCopyMatching(q as CFDictionary, &item) == errSecSuccess,
              let data = item as? Data else { return nil }

        return try? JSONDecoder().decode(Session.self, from: data)
    }

    func save(_ session: Session) {
        guard let data = try? JSONEncoder().encode(session) else { return }

        // 上書きは delete → add。SecItemUpdate だと不在時に失敗する
        SecItemDelete(query as CFDictionary)

        var q = query
        q[kSecValueData as String] = data
        q[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
        SecItemAdd(q as CFDictionary, nil)
    }

    func clear() {
        SecItemDelete(query as CFDictionary)
    }
}
