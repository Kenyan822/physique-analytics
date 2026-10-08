import Foundation

/// API が返したエラー。
enum APIError: Error, LocalizedError {
    /// path は**どこで起きたかを画面に出すため**。
    /// サーバの detail だけだと「invalid format」のように、
    /// 何のリクエストか分からないまま詰まる（実機で踏んだ）
    case http(status: Int, problem: Problem?, path: String = "")
    case transport(Error)
    case decoding(Error)

    var errorDescription: String? {
        switch self {
        case let .http(status, problem, path):
            // 何が起きたかが1行で分かるようにする。**どこで起きたかも**
            let what = problem.map { $0.detail ?? $0.title } ?? "HTTP \(status)"

            return path.isEmpty ? what : "\(what)（\(path) / \(status)）"
        case let .transport(e):
            return "通信できない: \(e.localizedDescription)"
        case let .decoding(e):
            return "応答を解釈できない: \(e.localizedDescription)"
        }
    }

    /// 「既にある」= 送信は届いたが応答が失われた。再送しても意味がない
    var isAlreadyRecorded: Bool {
        guard case let .http(status, problem, _) = self else { return false }
        guard status == 409 || status == 422 else { return false }

        let text = (problem?.detail ?? "") + (problem?.title ?? "")

        return text.contains("既にある") || text.contains("既に存在する")
    }
}

/// HTTP のやり取りだけを抽象化する。テストで差し替えるため。
protocol HTTPTransport: Sendable {
    func send(_ request: URLRequest) async throws -> (Data, HTTPURLResponse)
}

struct URLSessionTransport: HTTPTransport {
    let session: URLSession

    init(session: URLSession = .shared) {
        self.session = session
    }

    func send(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw APIError.transport(URLError(.badServerResponse))
        }

        return (data, http)
    }
}

/// API クライアント。
///
/// 型は `openapi.yaml` が正（ADR-0007）。Swift では
/// swift-openapi-generator を使う方針だが、まずは手書きの薄い層で動かし、
/// エンドポイントが増えてから生成に切り替える。
struct APIClient: Sendable {
    /// 呼び出しのたびにトークンを解決する。
    ///
    /// **作った時点の値を握らない。** アクセストークンは1時間で切れるので、
    /// 固定してしまうと1時間後から 401 になる。期限が近ければ取り直すのは
    /// AuthModel の仕事で、ここはそれを呼ぶだけ。
    typealias TokenProvider = @Sendable () async throws -> String?

    let baseURL: URL
    /// Supabase の JWT を返す。nil を返せば Authorization を付けない
    var tokenProvider: TokenProvider?
    var transport: HTTPTransport

    init(
        baseURL: URL,
        tokenProvider: TokenProvider? = nil,
        transport: HTTPTransport = URLSessionTransport()
    ) {
        self.baseURL = baseURL
        self.tokenProvider = tokenProvider
        self.transport = transport
    }

    /// 固定のトークンで作る。テストと、認証が要らない経路で使う
    init(baseURL: URL, token: String?, transport: HTTPTransport = URLSessionTransport()) {
        let provider: TokenProvider? = token.map { value in
            { @Sendable in value }
        }
        self.init(baseURL: baseURL, tokenProvider: provider, transport: transport)
    }

    // MARK: - 種目

    func listExercises(muscleGroup: MuscleGroup? = nil) async throws -> [Exercise] {
        var query: [URLQueryItem] = []
        if let mg = muscleGroup {
            query.append(URLQueryItem(name: "muscleGroup", value: mg.rawValue))
        }

        struct Response: Decodable { let items: [Exercise] }

        return try await request(Response.self, "GET", "/v1/exercises", query: query).items
    }

    /// 前回の実施内容（要件 T-02）。
    /// 今日やる想定の種目（要件 T-01 / #232）。
    ///
    /// **前回値も一緒に返る。** 行ごとに `lastPerformance` を叩くと
    /// 画面を開くたびに種目数ぶんの往復になる
    func todayRoutine(date: String? = nil) async throws -> TodayRoutine {
        let q = date.map { [URLQueryItem(name: "date", value: $0)] } ?? []

        return try await request(TodayRoutine.self, "GET", "v1/routines/today", query: q)
    }

    /// 前回の実施内容（要件 T-02）。
    ///
    /// **`before` を渡す。** 省くと今日を含めた最新が返り、
    /// 「前回」に今打った値が出る（#257・実機で踏んだ）
    func lastPerformance(exerciseId: UUID, before: String? = nil) async throws -> LastPerformance {
        try await request(
            LastPerformance.self, "GET",
            "/v1/exercises/\(escape(exerciseId.uuidString))/last-performance",
            query: before.map { [URLQueryItem(name: "before", value: $0)] } ?? []
        )
    }

    // MARK: - 記録

    func listSessions(from: String, to: String, limit: Int = 50) async throws -> [WorkoutSession] {
        struct Response: Decodable { let items: [WorkoutSession] }

        let query = [
            URLQueryItem(name: "from", value: from),
            URLQueryItem(name: "to", value: to),
            URLQueryItem(name: "limit", value: String(limit)),
        ]

        return try await request(Response.self, "GET", "/v1/workout-sessions", query: query).items
    }

    /// セッションを作る。同じ id での再送は冪等（ADR-0014）。
    func createSession(_ input: WorkoutSessionInput) async throws -> WorkoutSession {
        try await request(WorkoutSession.self, "POST", "/v1/workout-sessions", body: input)
    }

    func createSet(sessionId: UUID, _ input: WorkoutSetInput) async throws -> WorkoutSet {
        try await request(
            WorkoutSet.self, "POST",
            "/v1/workout-sessions/\(escape(sessionId.uuidString))/sets",
            body: input
        )
    }

    /// その日の種目リストを全置換する（#242）。
    ///
    /// **追加・削除・並べ替えを分けない。** ドラッグ中の中間状態で順序が壊れる
    func replaceSessionExercises(
        sessionId: UUID, exerciseIds: [UUID]
    ) async throws -> [SessionExercise] {
        struct Body: Encodable { let exerciseIds: [UUID] }
        struct Response: Decodable { let items: [SessionExercise] }

        return try await request(
            Response.self, "PUT",
            "/v1/workout-sessions/\(escape(sessionId.uuidString))/exercises",
            body: Body(exerciseIds: exerciseIds)
        ).items
    }

    func updateSet(id: UUID, _ input: WorkoutSetInput) async throws -> WorkoutSet {
        try await request(WorkoutSet.self, "PATCH",
                          "/v1/workout-sets/\(escape(id.uuidString))", body: input)
    }

    func deleteSet(id: UUID) async throws {
        try await requestNoContent("DELETE", "/v1/workout-sets/\(escape(id.uuidString))")
    }

    /// 日別の達成フラグ（#248）。**記録が無い日も行として返る**
    func streaks(from: String, to: String) async throws -> [StreakDay] {
        struct Response: Decodable { let items: [StreakDay] }

        return try await request(
            Response.self, "GET", "/v1/streaks",
            query: [.init(name: "from", value: from), .init(name: "to", value: to)]
        ).items
    }

    /// 写真から PFC を推定する（要件 N-06 / #253）。
    ///
    /// **結果は保存されていない。** 編集できる下書きとして返る。
    /// 鍵が未設定なら 503（課金は発生しない）
    func estimateMeal(image: Data, note: String?) async throws -> MealEstimate {
        let boundary = "physique-\(UUID().uuidString)"
        var body = Data()

        func append(_ s: String) { body.append(Data(s.utf8)) }

        append("--\(boundary)\r\n")
        append("Content-Disposition: form-data; name=\"image\"; filename=\"meal.jpg\"\r\n")
        append("Content-Type: image/jpeg\r\n\r\n")
        body.append(image)
        append("\r\n")

        // **量を添えると精度が大きく上がる**（写真だけでは食器のサイズが分からない）
        if let note, !note.trimmingCharacters(in: .whitespaces).isEmpty {
            append("--\(boundary)\r\n")
            append("Content-Disposition: form-data; name=\"note\"\r\n\r\n")
            append(note)
            append("\r\n")
        }
        append("--\(boundary)--\r\n")

        let data = try await sendMultipart(
            "/v1/meals/estimate", boundary: boundary, body: body)

        do {
            return try JSONDecoder().decode(MealEstimate.self, from: data)
        } catch {
            throw APIError.decoding(error)
        }
    }

    // MARK: - 食事（要件 N-01 / N-02 / N-05）

    func listMeals(from: String, to: String) async throws -> [Meal] {
        struct Response: Decodable { let items: [Meal] }

        return try await request(
            Response.self, "GET", "v1/meals",
            query: [.init(name: "from", value: from), .init(name: "to", value: to)]
        ).items
    }

    func createMeal(_ input: MealInput) async throws -> Meal {
        try await request(Meal.self, "POST", "v1/meals", body: input)
    }

    /// 記録を直す。**時刻を変えると区分も付け直される**（サーバが導出する）
    func updateMeal(id: UUID, _ input: MealInput) async throws -> Meal {
        try await request(Meal.self, "PATCH", "v1/meals/\(id.uuidString.lowercased())", body: input)
    }

    func deleteMeal(id: UUID) async throws {
        try await requestNoContent("DELETE", "v1/meals/\(id.uuidString.lowercased())")
    }

    /// 過去の記録から候補を引く（要件 N-02）。
    /// **食品マスタを持たない**ので、記録がそのままマスタになる
    func mealSuggestions(query: String = "", limit: Int = 20) async throws -> [MealSuggestion] {
        struct Response: Decodable { let items: [MealSuggestion] }

        return try await request(
            Response.self, "GET", "v1/meals/suggestions",
            query: [.init(name: "q", value: query), .init(name: "limit", value: String(limit))]
        ).items
    }

    /// その日の摂取目標（要件 N-05）。
    /// **フェーズ未登録だと 422 になる。** 呼ぶ側で「まだ出せない」として扱う
    func dailyTargets(date: String) async throws -> DailyTargets {
        try await request(DailyTargets.self, "GET", "v1/targets/\(date)")
    }

    // MARK: - 食品マスタ（要件 N-02 / ADR-0017）

    /// よく使う順。**引数つきの項目は components が入る**
    func foodItems(query: String = "") async throws -> [FoodItem] {
        struct Response: Decodable { let items: [FoodItem] }

        var q: [URLQueryItem] = []
        if !query.isEmpty { q.append(URLQueryItem(name: "q", value: query)) }

        return try await request(Response.self, "GET", "v1/food-items", query: q).items
    }

    func createFoodItem(_ input: FoodItemInput) async throws -> FoodItem {
        try await request(FoodItem.self, "POST", "v1/food-items", body: input)
    }

    /// 直す。**構成はまるごと置き換わる**
    func updateFoodItem(id: UUID, _ input: FoodItemInput) async throws -> FoodItem {
        try await request(FoodItem.self, "PATCH",
                          "v1/food-items/\(id.uuidString.lowercased())", body: input)
    }

    /// 使った回数を1つ増やす。一覧の並び順に効く。
    ///
    /// **失敗しても呼ぶ側は握ってよい。** 並び順が変わらないだけで、
    /// 入力そのものは済んでいる
    func markFoodItemUsed(id: UUID) async throws {
        try await requestNoContent("POST", "v1/food-items/\(id.uuidString.lowercased())/used")
    }

    func deleteFoodItem(id: UUID) async throws {
        try await requestNoContent("DELETE", "v1/food-items/\(id.uuidString.lowercased())")
    }

    /// 手で決めた摂取目標（要件 N-05）。設定していなければ nil
    func manualTargets() async throws -> ManualTargets? {
        struct Response: Decodable { let targets: ManualTargets? }

        return try await request(Response.self, "GET", "v1/targets/manual").targets
    }

    func putManualTargets(_ input: ManualTargets) async throws -> ManualTargets {
        try await request(ManualTargets.self, "PUT", "v1/targets/manual", body: input)
    }

    /// 消すと自動計算（A-02）に戻る
    func deleteManualTargets() async throws {
        try await requestNoContent("DELETE", "v1/targets/manual")
    }

    // MARK: - 体組成（要件 B-02 / B-03 / B-06）

    func listDailyMetrics(from: String, to: String) async throws -> [DailyMetrics] {
        struct Response: Decodable { let items: [DailyMetrics] }

        return try await request(
            Response.self, "GET", "v1/daily",
            query: [.init(name: "from", value: from), .init(name: "to", value: to)]
        ).items
    }

    func putDailyMetrics(_ input: DailyMetricsInput) async throws -> DailyMetrics {
        try await request(DailyMetrics.self, "PUT", "v1/daily", body: input)
    }

    /// 直近の周囲長。記録が無ければ nil（初回は無いのが正常）。
    func latestMeasurement() async throws -> BodyMeasurement? {
        struct Response: Decodable { let measurement: BodyMeasurement? }

        return try await request(Response.self, "GET", "v1/measurements/latest").measurement
    }

    func putMeasurement(_ input: BodyMeasurementInput) async throws -> BodyMeasurement {
        try await request(BodyMeasurement.self, "PUT", "v1/measurements", body: input)
    }

    // MARK: - 内部

    /// `-._~` は RFC 3986 の unreserved。**逃がさない。**
    /// UUID のハイフンまで `%2D` になると、ログもエラー文も読めなくなる
    private static let pathSafe = CharacterSet.alphanumerics.union(CharacterSet(charactersIn: "-._~"))

    private func escape(_ s: String) -> String {
        s.addingPercentEncoding(withAllowedCharacters: Self.pathSafe) ?? s
    }

    /// 本文を返さない経路（204）。読もうとすると decoding で落ちる
    private func requestNoContent(_ method: String, _ path: String) async throws {
        _ = try await sendRequest(method, path, query: [], body: Optional<Never>.none)
    }

    private func request<T: Decodable>(
        _ type: T.Type,
        _ method: String,
        _ path: String,
        query: [URLQueryItem] = [],
        body: (some Encodable)? = Optional<String>.none
    ) async throws -> T {
        let data = try await sendRequest(method, path, query: query, body: body)

        do {
            return try JSONDecoder().decode(T.self, from: data)
        } catch {
            throw APIError.decoding(error)
        }
    }

    /// 送って本文をそのまま返す。**デコードしない** ——
    /// 204 のように本文が無い応答もあるため、解釈は呼ぶ側に任せる
    /// multipart の送信（#253）。**JSON の経路と分ける。**
    /// 本文の組み立て方が違うだけなので、エラーの扱いは同じにする
    private func sendMultipart(
        _ path: String, boundary: String, body: Data
    ) async throws -> Data {
        guard let url = URLComponents(
            url: baseURL.appendingPathComponent(path), resolvingAgainstBaseURL: false
        )?.url else {
            throw APIError.transport(URLError(.badURL))
        }

        var req = URLRequest(url: url)
        req.httpMethod = "POST"
        if let token = try await tokenProvider?() {
            req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        req.setValue("multipart/form-data; boundary=\(boundary)",
                     forHTTPHeaderField: "Content-Type")
        req.httpBody = body

        let (data, http): (Data, HTTPURLResponse)
        do {
            (data, http) = try await transport.send(req)
        } catch let e as APIError {
            throw e
        } catch {
            throw APIError.transport(error)
        }

        guard (200..<300).contains(http.statusCode) else {
            let problem = try? JSONDecoder().decode(Problem.self, from: data)
            throw APIError.http(status: http.statusCode, problem: problem, path: url.path)
        }

        return data
    }

    private func sendRequest(
        _ method: String,
        _ path: String,
        query: [URLQueryItem],
        body: (some Encodable)?
    ) async throws -> Data {
        var components = URLComponents(
            url: baseURL.appendingPathComponent(path),
            resolvingAgainstBaseURL: false
        )
        if !query.isEmpty {
            components?.queryItems = query
        }
        guard let url = components?.url else {
            throw APIError.transport(URLError(.badURL))
        }

        var req = URLRequest(url: url)
        req.httpMethod = method
        if let token = try await tokenProvider?() {
            req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        if let body {
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
            do {
                req.httpBody = try JSONEncoder().encode(body)
            } catch {
                throw APIError.decoding(error)
            }
        }

        let (data, http): (Data, HTTPURLResponse)
        do {
            (data, http) = try await transport.send(req)
        } catch let e as APIError {
            throw e
        } catch {
            throw APIError.transport(error)
        }

        guard (200..<300).contains(http.statusCode) else {
            // problem+json で返ってこないこともある（LB の 502 など）。
            // そこで落ちるとエラーの原因が「解釈できない」にすり替わる
            let problem = try? JSONDecoder().decode(Problem.self, from: data)
            throw APIError.http(
                status: http.statusCode, problem: problem,
                path: url.path
            )
        }

        return data
    }
}
