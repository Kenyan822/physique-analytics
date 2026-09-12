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
    let baseURL: URL
    /// Supabase の JWT。無ければ Authorization を付けない
    var token: String?
    var transport: HTTPTransport

    init(baseURL: URL, token: String? = nil, transport: HTTPTransport = URLSessionTransport()) {
        self.baseURL = baseURL
        self.token = token
        self.transport = transport
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

    // MARK: - 内部

    private func escape(_ s: String) -> String {
        s.addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? s
    }

    private func request<T: Decodable>(
        _ type: T.Type,
        _ method: String,
        _ path: String,
        query: [URLQueryItem] = [],
        body: (some Encodable)? = Optional<String>.none
    ) async throws -> T {
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
        if let token {
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

        do {
            return try JSONDecoder().decode(T.self, from: data)
        } catch {
            throw APIError.decoding(error)
        }
    }
}
