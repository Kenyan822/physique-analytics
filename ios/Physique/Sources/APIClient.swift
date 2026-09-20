import Foundation

/// API が返したエラー。
enum APIError: Error, LocalizedError {
    case http(status: Int, problem: Problem?)
    case transport(Error)
    case decoding(Error)

    var errorDescription: String? {
        switch self {
        case let .http(status, problem):
            // 何が起きたかが1行で分かるようにする
            if let p = problem {
                return p.detail ?? p.title
            }
            return "HTTP \(status)"
        case let .transport(e):
            return "通信できない: \(e.localizedDescription)"
        case let .decoding(e):
            return "応答を解釈できない: \(e.localizedDescription)"
        }
    }

    /// 「既にある」= 送信は届いたが応答が失われた。再送しても意味がない
    var isAlreadyRecorded: Bool {
        guard case let .http(status, problem) = self else { return false }
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
    func lastPerformance(exerciseId: UUID) async throws -> LastPerformance {
        try await request(
            LastPerformance.self, "GET",
            "/v1/exercises/\(escape(exerciseId.uuidString))/last-performance"
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

    private func escape(_ s: String) -> String {
        s.addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? s
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
            throw APIError.http(status: http.statusCode, problem: problem)
        }

        return data
    }
}
