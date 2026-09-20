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

@Suite("トークンの渡し方")
struct APIClientTokenTests {
    @Test("固定のトークンをそのまま付ける")
    func staticToken() async throws {
        let t = FakeTransport(json: #"{"items":[]}"#)
        let api = APIClient(baseURL: base, token: "jwt-fixed", transport: t)

        _ = try await api.listExercises()

        #expect(t.requests.first?.value(forHTTPHeaderField: "Authorization") == "Bearer jwt-fixed")
    }

    @Test("**呼び出しのたびにトークンを取り直せる**")
    func tokenProvider() async throws {
        // アクセストークンは1時間で切れる。作った時点の値を握り続けると
        // 1時間後から 401 になる
        let t = FakeTransport(json: #"{"items":[]}"#)
        let counter = Counter()
        let api = APIClient(
            baseURL: base,
            tokenProvider: { await counter.next() },
            transport: t
        )

        _ = try await api.listExercises()
        _ = try await api.listExercises()

        #expect(t.requests.map { $0.value(forHTTPHeaderField: "Authorization") }
            == ["Bearer jwt-1", "Bearer jwt-2"])
    }

    @Test("トークンが無ければ Authorization を付けない")
    func noToken() async throws {
        // /health のように認証が要らない経路がある
        let t = FakeTransport(json: #"{"items":[]}"#)
        let api = APIClient(baseURL: base, transport: t)

        _ = try await api.listExercises()

        #expect(t.requests.first?.value(forHTTPHeaderField: "Authorization") == nil)
    }
}

private actor Counter {
    private var n = 0
    func next() -> String {
        n += 1
        return "jwt-\(n)"
    }
}

@Suite("食事の更新")
struct MealUpdateTests {
    @Test("PATCH で送る")
    func patches() async throws {
        let t = FakeTransport(json: #"""
        {"id":"11111111-1111-1111-1111-111111111111","date":"2026-09-19","at":"20:10",
         "slot":"夕食","name":"鶏むね","kcal":220,"source":"manual",
         "createdAt":"2026-09-19T10:00:00Z","updatedAt":"2026-09-19T11:00:00Z"}
        """#)
        let api = APIClient(baseURL: base, transport: t)

        let id = UUID(uuidString: "11111111-1111-1111-1111-111111111111")!
        var input = MealInput(date: "2026-09-19", name: "鶏むね")
        input.at = "20:10"

        let updated = try await api.updateMeal(id: id, input)

        #expect(updated.at == "20:10")
        let req = try #require(t.requests.first)
        #expect(req.httpMethod == "PATCH")
        #expect(req.url?.path == "/v1/meals/11111111-1111-1111-1111-111111111111")
    }
}

@Suite("食品マスタの API")
struct FoodItemAPITests {
    @Test("一覧は items を取り出す")
    func list() async throws {
        let t = FakeTransport(json: #"""
        {"items":[{"id":"11111111-1111-1111-1111-111111111111","name":"ゆで卵",
          "proteinG":6.5,"fatG":5.2,"carbG":0.2,"components":[],"usedCount":3,
          "createdAt":"2026-09-21T00:00:00Z","updatedAt":"2026-09-21T00:00:00Z"}]}
        """#)
        let api = APIClient(baseURL: base, transport: t)

        let items = try await api.foodItems()

        #expect(items.count == 1)
        #expect(items[0].name == "ゆで卵")
        // **引数なしは空配列**（ADR-0017 の既定の経路）
        #expect(items[0].components.isEmpty)
    }

    @Test("構成を読める")
    func components() async throws {
        let t = FakeTransport(json: #"""
        {"items":[{"id":"11111111-1111-1111-1111-111111111111","name":"プロテイン",
          "components":[{"name":"量","unit":"g","basisAmount":30,"defaultAmount":30,
            "proteinG":24,"fatG":1.5,"carbG":2}],"usedCount":0,
          "createdAt":"2026-09-21T00:00:00Z","updatedAt":"2026-09-21T00:00:00Z"}]}
        """#)
        let api = APIClient(baseURL: base, transport: t)

        let items = try await api.foodItems()
        let c = try #require(items.first?.components.first)

        #expect(c.name == "量")
        #expect(c.basisAmount == 30)
        #expect(c.proteinG == 24)
    }

    @Test("名前で絞るとクエリに載る")
    func filters() async throws {
        let t = FakeTransport(json: #"{"items":[]}"#)
        let api = APIClient(baseURL: base, transport: t)

        _ = try await api.foodItems(query: "プロテイン")

        let url = try #require(t.requests.first?.url)
        #expect(url.query?.contains("q=") == true)
    }

    @Test("空の絞り込みはクエリを付けない")
    func noQuery() async throws {
        let t = FakeTransport(json: #"{"items":[]}"#)
        let api = APIClient(baseURL: base, transport: t)

        _ = try await api.foodItems()

        #expect(try #require(t.requests.first?.url).query == nil)
    }

    @Test("登録は POST で送る")
    func create() async throws {
        let t = FakeTransport(json: #"""
        {"id":"11111111-1111-1111-1111-111111111111","name":"ゆで卵","components":[],
         "usedCount":0,"createdAt":"2026-09-21T00:00:00Z","updatedAt":"2026-09-21T00:00:00Z"}
        """#, status: 201)
        let api = APIClient(baseURL: base, transport: t)

        _ = try await api.createFoodItem(
            FoodItemInput(name: "ゆで卵", proteinG: 6.5, fatG: 5.2, carbG: 0.2))

        let req = try #require(t.requests.first)
        #expect(req.httpMethod == "POST")
        #expect(req.url?.path == "/v1/food-items")
    }
}
