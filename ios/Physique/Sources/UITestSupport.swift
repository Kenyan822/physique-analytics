#if DEBUG
import Foundation

/// UI テストのときだけ使う、通信もログインもしない組み立て。
///
/// **Release ビルドには入らない**（`#if DEBUG`）。本番の経路は変えず、
/// 既存の protocol（`HTTPTransport` / `SessionStore`）に偽物を挿すだけ。
///
/// 起動引数で切り替える。
///
///     app.launchArguments = ["-uiTesting"]
enum UITestSupport {
    static var isActive: Bool {
        ProcessInfo.processInfo.arguments.contains("-uiTesting")
    }

    /// ログイン済みの状態を作る。**Keychain を汚さない**ためメモリに置く
    @MainActor
    static func makeAuth() -> AuthModel {
        let store = MemorySessionStore()
        store.save(Session(
            accessToken: "test", refreshToken: "test",
            expiresAt: Date().addingTimeInterval(3600)
        ))

        return AuthModel(
            auth: AuthClient(url: URL(string: "https://test.invalid")!,
                             anonKey: "test", transport: StubTransport()),
            store: store
        )
    }

    static func makeAPI() -> APIClient {
        APIClient(baseURL: URL(string: "https://test.invalid")!,
                  token: "test", transport: StubTransport())
    }

    /// **本物の CoreLocation を挿さない。** システムのダイアログが出ると
    /// テストから押せない。決め打ちの場所を返す（要件 N-08）
    static func makeLocation() -> LocationSource { StubLocation() }
}

/// いつでも許可して、決まった場所を返す偽物。
private final class StubLocation: LocationSource, @unchecked Sendable {
    private let lock = NSLock()
    private var granted = false

    var permission: LocationPermission {
        lock.withLock { granted ? .granted : .notDetermined }
    }

    func request() async -> LocationPermission {
        lock.withLock { granted = true }

        return .granted
    }

    /// 適当な座標。**実在の場所を書かない**（公開リポジトリ）
    func current() async -> Coordinate? {
        permission == .granted ? Coordinate(lat: 35.0, lng: 139.0) : nil
    }
}

/// メモリ上のセッション置き場。テストが Keychain を触らないようにする。
private final class MemorySessionStore: SessionStore, @unchecked Sendable {
    private let lock = NSLock()
    private var session: Session?

    func load() -> Session? {
        lock.lock(); defer { lock.unlock() }

        return session
    }

    func save(_ s: Session) {
        lock.lock(); defer { lock.unlock() }
        session = s
    }

    func clear() {
        lock.lock(); defer { lock.unlock() }
        session = nil
    }
}

/// 決め打ちの応答を返す偽の通信。
///
/// **記録した内容を覚える。** 「記録したら一覧に出る」を確かめたいので、
/// 毎回同じものを返すだけでは足りない。
private final class StubTransport: HTTPTransport, @unchecked Sendable {
    private let lock = NSLock()
    private var meals: [[String: Any]] = []
    private var manualTargets: [String: Any]?
    private var foodItems: [[String: Any]] = []

    func send(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
        let path = request.url?.path ?? ""
        let method = request.httpMethod ?? "GET"
        let body = request.httpBody.flatMap {
            try? JSONSerialization.jsonObject(with: $0) as? [String: Any]
        } ?? [:]

        // **`lock.lock()` を async の中で呼べない**（Swift 6）。
        // withLock なら同期のスコープに閉じるので通る
        let (json, status) = lock.withLock {
            respond(path: path, method: method, body: body)
        }

        return (
            try JSONSerialization.data(withJSONObject: json),
            HTTPURLResponse(url: request.url!, statusCode: status,
                            httpVersion: nil, headerFields: nil)!
        )
    }

    private func respond(
        path: String, method: String, body: [String: Any]
    ) -> ([String: Any], Int) {
        switch (method, path) {
        case ("GET", let p) where p.hasSuffix("/v1/meals"):
            return (["items": meals], 200)

        case ("POST", let p) where p.hasSuffix("/v1/meals"):
            var meal = body
            meal["id"] = UUID().uuidString.lowercased()
            meal["source"] = "manual"
            meal["kcal"] = kcal(from: body)
            meals.append(meal)

            return (meal, 201)

        case ("GET", let p) where p.hasSuffix("/v1/targets/manual"):
            return (["targets": manualTargets as Any], 200)

        case ("PUT", let p) where p.hasSuffix("/v1/targets/manual"):
            var t = body
            t["kcal"] = kcal(from: body)
            manualTargets = t

            return (t, 200)

        case ("DELETE", let p) where p.hasSuffix("/v1/targets/manual"):
            manualTargets = nil

            return ([:], 204)

        case ("GET", let p) where p.contains("/v1/targets/"):
            guard let t = manualTargets else {
                return (problem("目標を出せない: フェーズが1つも登録されていない"), 422)
            }

            return ([
                "date": String(p.split(separator: "/").last ?? ""),
                "targetSource": "manual",
                "target": t,
                "consumed": totals(),
                "remaining": remaining(from: t),
            ], 200)

        case ("GET", let p) where p.hasSuffix("/v1/food-items"):
            return (["items": foodItems], 200)

        case ("POST", let p) where p.hasSuffix("/v1/food-items"):
            var item = body
            item["id"] = UUID().uuidString.lowercased()
            item["components"] = body["components"] ?? []
            item["usedCount"] = 0
            item["createdAt"] = "2026-09-21T00:00:00Z"
            item["updatedAt"] = "2026-09-21T00:00:00Z"
            foodItems.append(item)

            return (item, 201)

        case ("PATCH", let p) where p.contains("/v1/food-items/"):
            guard let i = foodItems.firstIndex(where: { $0["id"] as? String == lastPath(p) }) else {
                return ([:], 404)
            }
            var item = foodItems[i]
            for (k, v) in body { item[k] = v }
            // **引数は丸ごと置き換わる。** サーバ側も同じ（replaceComponents）
            item["components"] = body["components"] ?? []
            foodItems[i] = item

            return (item, 200)

        case ("DELETE", let p) where p.contains("/v1/food-items/"):
            foodItems.removeAll { $0["id"] as? String == lastPath(p) }

            return ([:], 204)

        case ("GET", let p) where p.hasSuffix("/v1/meal-sets"):
            return (["items": []], 200)

        default:
            return (["items": []], 200)
        }
    }

    private func lastPath(_ p: String) -> String {
        String(p.split(separator: "/").last ?? "")
    }

    private func kcal(from m: [String: Any]) -> Int {
        let p = (m["proteinG"] as? Double) ?? 0
        let f = (m["fatG"] as? Double) ?? 0
        let c = (m["carbG"] as? Double) ?? 0

        return Int((p * 4 + f * 9 + c * 4).rounded())
    }

    private func totals() -> [String: Any] {
        [
            "kcal": meals.compactMap { $0["kcal"] as? Int }.reduce(0, +),
            "proteinG": sum("proteinG"),
            "fatG": sum("fatG"),
            "carbG": sum("carbG"),
        ]
    }

    private func sum(_ key: String) -> Double {
        meals.compactMap { $0[key] as? Double }.reduce(0, +)
    }

    private func remaining(from t: [String: Any]) -> [String: Any] {
        [
            "kcal": ((t["kcal"] as? Int) ?? 0) - (totals()["kcal"] as? Int ?? 0),
            "proteinG": ((t["proteinG"] as? Double) ?? 0) - sum("proteinG"),
            "fatG": ((t["fatG"] as? Double) ?? 0) - sum("fatG"),
            "carbG": ((t["carbG"] as? Double) ?? 0) - sum("carbG"),
        ]
    }

    private func problem(_ detail: String) -> [String: Any] {
        ["type": "about:blank", "title": "目標を出せない", "status": 422, "detail": detail]
    }
}
#endif
