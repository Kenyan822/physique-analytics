import Foundation
import Testing

@testable import PhysiqueCore

/// 記録した URLRequest を返すだけの偽 transport。
/// 実 API に繋ぐとテストがネットワークと DB の状態に依存する。
final class FakeTransport: HTTPTransport, @unchecked Sendable {
    private(set) var requests: [URLRequest] = []
    var responses: [(Data, Int)] = []
    var error: Error?

    init(json: String = "{}", status: Int = 200) {
        responses = [(Data(json.utf8), status)]
    }

    func send(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
        requests.append(request)
        if let error { throw error }

        let (data, status) = responses.count > 1 ? responses.removeFirst() : responses[0]
        let http = HTTPURLResponse(
            url: request.url!, statusCode: status,
            httpVersion: nil, headerFields: ["Content-Type": "application/json"]
        )!

        return (data, http)
    }
}

private let base = URL(string: "http://api.test")!

@Suite("APIClient")
struct APIClientTests {
    @Test("種目の一覧を組み立てる")
    func listExercises() async throws {
        let t = FakeTransport(json: """
        {"items":[{"id":"11111111-1111-1111-1111-111111111111","name":"ベンチプレス","muscleGroup":"胸","isCompound":true}]}
        """)
        let api = APIClient(baseURL: base, transport: t)

        let items = try await api.listExercises()

        #expect(items.count == 1)
        #expect(items[0].name == "ベンチプレス")
        #expect(items[0].muscleGroup == .chest)

        let url = try #require(t.requests.first?.url)
        #expect(url.path == "/v1/exercises")
        #expect(url.query == nil)
    }

    @Test("部位で絞ると日本語がエンコードされる")
    func filterByMuscleGroup() async throws {
        let t = FakeTransport(json: #"{"items":[]}"#)
        let api = APIClient(baseURL: base, transport: t)

        _ = try await api.listExercises(muscleGroup: .chest)

        let url = try #require(t.requests.first?.url)
        let items = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems
        #expect(items?.first(where: { $0.name == "muscleGroup" })?.value == "胸")
        // 生で載せると環境によって壊れる
        #expect(url.absoluteString.contains("%E8%83%B8"))
    }

    @Test("トークンがあれば Authorization を付ける")
    func withToken() async throws {
        let t = FakeTransport(json: #"{"items":[]}"#)
        let api = APIClient(baseURL: base, token: "tok", transport: t)

        _ = try await api.listExercises()

        #expect(t.requests.first?.value(forHTTPHeaderField: "Authorization") == "Bearer tok")
    }

    @Test("トークンが無ければ付けない")
    func withoutToken() async throws {
        let t = FakeTransport(json: #"{"items":[]}"#)
        let api = APIClient(baseURL: base, transport: t)

        _ = try await api.listExercises()

        #expect(t.requests.first?.value(forHTTPHeaderField: "Authorization") == nil)
    }

    @Test("problem+json を APIError にする")
    func problemJSON() async throws {
        let t = FakeTransport(
            json: #"{"type":"about:blank","title":"種目が見つからない","status":404}"#,
            status: 404
        )
        let api = APIClient(baseURL: base, transport: t)

        await #expect(throws: APIError.self) {
            _ = try await api.lastPerformance(exerciseId: UUID())
        }

        do {
            _ = try await api.lastPerformance(exerciseId: UUID())
        } catch let e as APIError {
            guard case let .http(status, problem) = e else {
                Issue.record("http ではない: \(e)")
                return
            }
            #expect(status == 404)
            #expect(problem?.title == "種目が見つからない")
            // メッセージだけ見ても何が起きたか分かる
            #expect(e.errorDescription == "種目が見つからない")
        }
    }

    /// LB の 502 など、problem+json で返ってこないこともある
    @Test("problem+json でないエラーでも落ちない")
    func nonProblemError() async throws {
        let t = FakeTransport(json: "<html>502</html>", status: 502)
        let api = APIClient(baseURL: base, transport: t)

        do {
            _ = try await api.listExercises()
            Issue.record("エラーを期待したが返ってきた")
        } catch let e as APIError {
            guard case let .http(status, problem) = e else {
                Issue.record("http ではない: \(e)")
                return
            }
            #expect(status == 502)
            #expect(problem == nil)
        }
    }

    @Test("POST は JSON として送る")
    func postJSON() async throws {
        let t = FakeTransport(json: """
        {"id":"22222222-2222-2222-2222-222222222222","date":"2026-09-13","sets":[]}
        """, status: 201)
        let api = APIClient(baseURL: base, transport: t)

        _ = try await api.createSession(WorkoutSessionInput(date: "2026-09-13"))

        let req = try #require(t.requests.first)
        #expect(req.httpMethod == "POST")
        #expect(req.value(forHTTPHeaderField: "Content-Type") == "application/json")

        let body = try #require(req.httpBody)
        let decoded = try JSONSerialization.jsonObject(with: body) as? [String: Any]
        #expect(decoded?["date"] as? String == "2026-09-13")
    }

    /// 送信は届いたが応答が失われた場合。再送しても意味がない
    @Test("「既にある」を判別する")
    func alreadyRecorded() {
        let conflict = APIError.http(
            status: 422,
            problem: Problem(type: "about:blank", title: "入力が仕様に合わない", status: 422,
                             detail: "セット番号 1 は既にある: 既に存在する")
        )
        #expect(conflict.isAlreadyRecorded)

        let notFound = APIError.http(
            status: 404,
            problem: Problem(type: "about:blank", title: "見つからない", status: 404, detail: nil)
        )
        #expect(!notFound.isAlreadyRecorded)
    }

    @Test("通信エラーを包む")
    func transportError() async {
        let t = FakeTransport()
        t.error = URLError(.notConnectedToInternet)
        let api = APIClient(baseURL: base, transport: t)

        await #expect(throws: APIError.self) {
            _ = try await api.listExercises()
        }
    }
}
