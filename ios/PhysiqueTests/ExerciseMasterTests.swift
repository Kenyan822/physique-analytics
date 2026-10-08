import Foundation
import Testing

@testable import PhysiqueCore

private let base = URL(string: "http://api.test")!

private let created = #"""
{"id":"aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee","name":"ケーブルクロスオーバー",
 "muscleGroup":"胸","isCompound":false}
"""#

@Suite("種目マスタに足す")
struct CreateExerciseAPITests {
    @Test("POST /v1/exercises を叩く")
    func posts() async throws {
        let t = FakeTransport(json: created, status: 201)
        let api = APIClient(baseURL: base, transport: t)

        let got = try await api.createExercise(
            ExerciseInput(name: "ケーブルクロスオーバー", muscleGroup: .chest, isCompound: false))

        #expect(got.name == "ケーブルクロスオーバー")
        let req = try #require(t.requests.last)
        #expect(req.httpMethod == "POST")
        #expect(req.url?.path == "/v1/exercises")

        let body = try JSONSerialization.jsonObject(with: #require(req.httpBody)) as! [String: Any]
        #expect(body["name"] as? String == "ケーブルクロスオーバー")
        #expect(body["muscleGroup"] as? String == "胸")
        #expect(body["isCompound"] as? Bool == false)
    }

    @Test("論理削除を叩く")
    func deletes() async throws {
        let t = FakeTransport(json: "{}", status: 204)
        let api = APIClient(baseURL: base, transport: t)
        let id = UUID()

        try await api.deleteExercise(id: id)

        let req = try #require(t.requests.last)
        #expect(req.httpMethod == "DELETE")
        // UUID は大小区別しない。既存の種目エンドポイントと同じ形（そのまま）
        #expect(req.url?.path == "/v1/exercises/\(id.uuidString)")
    }

    @Test("**同じ名前は 409**")
    func duplicate() async throws {
        let p = #"{"type":"about:blank","title":"既にある","status":409,"detail":"同じ名前の種目がある"}"#
        let t = FakeTransport(json: p, status: 409)
        let api = APIClient(baseURL: base, transport: t)

        await #expect(throws: APIError.self) {
            _ = try await api.createExercise(
                ExerciseInput(name: "ベンチプレス", muscleGroup: .chest))
        }
    }
}

@Suite("登録したらすぐ使える")
@MainActor
struct RegisterExerciseTests {
    private let bench = UUID(uuidString: "22222222-2222-2222-2222-222222222222")!

    private func loaded() async -> (LogModel, FakeTransport) {
        let t = FakeTransport()
        t.responses = [
            (Data(#"{"items":[{"id":"22222222-2222-2222-2222-222222222222","name":"ベンチプレス","muscleGroup":"胸"}]}"#.utf8), 200),
            (Data(#"{"items":[{"id":"55555555-5555-5555-5555-555555555555","date":"2026-10-08","sets":[],"exercises":[]}]}"#.utf8), 200),
            (Data(#"{"date":"2026-10-08"}"#.utf8), 200),
        ]
        let m = LogModel(api: APIClient(baseURL: base, transport: t),
                         queue: PendingQueue(store: MemoryPendingStore()),
                         date: "2026-10-08", today: "2026-10-08")
        await m.load()

        return (m, t)
    }

    @Test("**登録したら一覧に出て、その日のリストにも入る**")
    func registersAndUses() async {
        let (m, t) = await loaded()
        t.responses = [
            (Data(created.utf8), 201),                 // createExercise
            (Data(#"{"items":[]}"#.utf8), 200),        // replaceSessionExercises
        ]

        await m.registerExercise(name: "ケーブルクロスオーバー", muscleGroup: .chest,
                                 isCompound: false)

        #expect(m.exercises.contains { $0.name == "ケーブルクロスオーバー" })
        #expect(m.rows.contains { $0.name == "ケーブルクロスオーバー" })
        // 登録したらそのまま打てる
        #expect(m.selectedExerciseId == m.rows.last?.exerciseId)
    }

    @Test("**名前が空なら送らない**")
    func rejectsEmptyName() async {
        let (m, t) = await loaded()
        let n = t.requests.count

        await m.registerExercise(name: "  ", muscleGroup: .chest, isCompound: false)

        #expect(t.requests.count == n)
    }

    @Test("失敗したら一覧に足さない")
    func failureKeepsList() async {
        let (m, t) = await loaded()
        let before = m.exercises.count
        let p = #"{"type":"about:blank","title":"既にある","status":409}"#
        t.responses = [(Data(p.utf8), 409)]

        await m.registerExercise(name: "ベンチプレス", muscleGroup: .chest, isCompound: false)

        #expect(m.exercises.count == before)
        #expect(m.showError)
    }
}
