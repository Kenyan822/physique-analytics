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
    private var sessions: [[String: Any]] = []

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
        // MARK: 筋トレ（#232）

        case ("GET", let p) where p.hasSuffix("/v1/exercises"):
            return (["items": Self.exercises], 200)

        case ("GET", let p) where p.contains("/last-performance"):
            // **前回のセッション**を返す。今日の記録は含めない（#257）
            return ([
                "exerciseId": lastPath(p, drop: 1),
                "date": "2026-10-01",
                "estimatedOneRm": 102.5,
                "sets": [
                    ["id": "88888888-0000-0000-0000-000000000001",
                     "sessionId": "66666666-6666-6666-6666-666666666666",
                     "exerciseId": lastPath(p, drop: 1),
                     "setNo": 1, "weightKg": 80, "reps": 8, "rir": 2],
                ],
            ], 200)

        case ("GET", let p) where p.hasSuffix("/v1/routines/today"):
            return (Self.routine, 200)

        case ("GET", let p) where p.hasSuffix("/v1/workout-sessions"):
            return (["items": sessions], 200)

        case ("POST", let p) where p.hasSuffix("/v1/workout-sessions"):
            var s = body
            s["id"] = UUID().uuidString.lowercased()
            s["sets"] = []
            s["exercises"] = []
            sessions.append(s)

            return (s, 201)

        // その日の種目リストを全置換する（#242）
        case ("PUT", let p) where p.hasSuffix("/exercises"):
            let ids = (body["exerciseIds"] as? [String]) ?? []
            let byId = Dictionary(uniqueKeysWithValues:
                Self.exercises.map { ($0["id"] as? String ?? "", $0) })
            let items = ids.enumerated().compactMap { i, id -> [String: Any]? in
                guard let e = byId[id] else { return nil }

                return ["exerciseId": id, "exerciseName": e["name"] as Any,
                        "muscleGroup": e["muscleGroup"] as Any, "itemOrder": i + 1]
            }
            if let i = sessions.firstIndex(where: { $0["id"] as? String == lastPath(p, drop: 1) }) {
                sessions[i]["exercises"] = items
            }

            return (["items": items], 200)

        case ("PATCH", let p) where p.contains("/v1/workout-sets/"):
            return (patchSet(id: lastPath(p), with: body), 200)

        case ("DELETE", let p) where p.contains("/v1/workout-sets/"):
            for i in sessions.indices {
                var sets = (sessions[i]["sets"] as? [[String: Any]]) ?? []
                sets.removeAll { $0["id"] as? String == lastPath(p) }
                sessions[i]["sets"] = sets
            }

            return ([:], 204)

        case ("POST", let p) where p.hasSuffix("/sets"):
            let sessionId = lastPath(p, drop: 1)
            var set = body
            set["id"] = (body["id"] as? String) ?? UUID().uuidString.lowercased()
            set["sessionId"] = sessionId
            if let i = sessions.firstIndex(where: { $0["id"] as? String == sessionId }) {
                var sets = (sessions[i]["sets"] as? [[String: Any]]) ?? []
                sets.append(set)
                sessions[i]["sets"] = sets
            }

            return (set, 201)

        // 日別の達成（#248）。from〜to の全日を返す
        case ("GET", let p) where p.hasSuffix("/v1/streaks"):
            return (["items": Self.streakDays], 200)

        // MARK: 食事

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

    /// 判定できない日（null）も混ぜる。**未達と見分けられるか**を見たい
    private static var streakDays: [[String: Any]] {
        (1...28).map { d in
            [
                "date": String(format: "2026-10-%02d", d),
                "mealGoalMet": d % 5 == 0 ? NSNull() : (d % 3 != 0),
                "trained": d % 2 == 0,
            ]
        }
    }

    private func patchSet(id: String, with body: [String: Any]) -> [String: Any] {
        for i in sessions.indices {
            var sets = (sessions[i]["sets"] as? [[String: Any]]) ?? []
            guard let j = sets.firstIndex(where: { $0["id"] as? String == id }) else { continue }
            for (k, v) in body { sets[j][k] = v }
            sessions[i]["sets"] = sets

            return sets[j]
        }

        return [:]
    }

    /// 末尾から `drop` 個手前のセグメント。`.../{id}/sets` の id を取るのに使う
    private func lastPath(_ p: String, drop: Int = 0) -> String {
        let parts = p.split(separator: "/")
        let i = parts.count - 1 - drop
        guard parts.indices.contains(i) else { return "" }

        return String(parts[i])
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

    // MARK: - 決め打ちのデータ

    /// **id は固定。** 毎回作り直すと、開き直したときに選択が外れる
    private static let ids = (1...6).map { String(format: "00000000-0000-4000-8000-%012d", $0) }

    /// 部位で分かれていることを見るので、**最低2部位**入れる
    private static var exercises: [[String: Any]] { [
        ["id": ids[0], "name": "ベンチプレス", "muscleGroup": "胸", "isCompound": true],
        ["id": ids[1], "name": "ダンベルフライ", "muscleGroup": "胸", "isCompound": false],
        ["id": ids[2], "name": "スクワット", "muscleGroup": "大腿四頭", "isCompound": true],
        ["id": ids[3], "name": "レッグエクステンション", "muscleGroup": "大腿四頭", "isCompound": false],
    ] }

    private static var routine: [String: Any] { [
        "date": "2026-09-21",
        "routineName": "テスト",
        "todayOrder": 1,
        "days": [
            [
                "dayOrder": 1, "templateId": ids[4], "templateName": "胸",
                "items": [
                    [
                        "exerciseId": ids[0], "exerciseName": "ベンチプレス",
                        "muscleGroup": "胸", "order": 1, "targetSets": 5,
                        "targetRepsMin": 6, "targetRepsMax": 10,
                    ],
                    [
                        "exerciseId": ids[1], "exerciseName": "ダンベルフライ",
                        "muscleGroup": "胸", "order": 2, "targetSets": 3,
                    ],
                ],
            ],
            [
                "dayOrder": 2, "templateId": ids[5], "templateName": "脚",
                "items": [
                    [
                        "exerciseId": ids[2], "exerciseName": "スクワット",
                        "muscleGroup": "大腿四頭", "order": 1, "targetSets": 4,
                    ],
                ],
            ],
        ],
    ] }

    private func problem(_ detail: String) -> [String: Any] {
        ["type": "about:blank", "title": "目標を出せない", "status": 422, "detail": detail]
    }
}
#endif
