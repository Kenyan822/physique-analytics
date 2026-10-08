import Foundation
import Testing

@testable import PhysiqueCore

private let base = URL(string: "http://api.test")!

/// 今日の想定（胸の日）
private let today = #"""
{"date":"2026-09-28","routineName":"6日サイクル","todayOrder":1,
 "days":[{"dayOrder":1,"templateId":"11111111-1111-1111-1111-111111111111",
   "templateName":"胸","items":[
     {"exerciseId":"22222222-2222-2222-2222-222222222222","exerciseName":"ベンチプレス",
      "muscleGroup":"胸","order":1,"targetSets":5,"targetRepsMin":6,"targetRepsMax":10,
      "lastDate":"2026-09-20","lastWeightKg":80,"lastReps":8,"lastRir":2,"doneToday":false},
     {"exerciseId":"33333333-3333-3333-3333-333333333333","exerciseName":"ディップス",
      "muscleGroup":"胸","order":2,"targetSets":4,"doneToday":true}]}]}
"""#

/// ルーティン未登録
private let empty = #"{"date":"2026-09-28"}"#

@Suite("今日のルーティン")
struct TodayRoutineTests {
    @Test("読める")
    func decodes() throws {
        let got = try JSONDecoder().decode(TodayRoutine.self, from: Data(today.utf8))

        #expect(got.routineName == "6日サイクル")
        #expect(got.totalDays == 1)
        #expect(got.today?.templateName == "胸")
        #expect(got.today?.items.count == 2)
    }

    @Test("前回値が入る")
    func lastValues() throws {
        let got = try JSONDecoder().decode(TodayRoutine.self, from: Data(today.utf8))
        let bench = try #require(got.today?.items.first)

        #expect(bench.lastWeightKg == 80)
        #expect(bench.lastReps == 8)
        #expect(bench.lastRir == 2)
    }

    @Test("**未登録でも読める**")
    func emptyRoutine() throws {
        let got = try JSONDecoder().decode(TodayRoutine.self, from: Data(empty.utf8))

        #expect(got.today == nil)
        #expect(got.routineName == nil)
    }

    @Test("前回が無ければ nil")
    func noLast() throws {
        let got = try JSONDecoder().decode(TodayRoutine.self, from: Data(today.utf8))
        let dips = try #require(got.today?.items.last)

        #expect(dips.lastWeightKg == nil)
        #expect(dips.lastDate == nil)
    }

    @Test("今日やったかが分かる")
    func doneToday() throws {
        let got = try JSONDecoder().decode(TodayRoutine.self, from: Data(today.utf8))

        #expect(got.today?.items.first?.doneToday == false)
        #expect(got.today?.items.last?.doneToday == true)
    }

    @Test("前回の表示文を作れる")
    func lastLabel() throws {
        let got = try JSONDecoder().decode(TodayRoutine.self, from: Data(today.utf8))
        let bench = try #require(got.today?.items.first)
        let dips = try #require(got.today?.items.last)

        #expect(bench.lastLabel == "前回 80×8 RIR2")
        // **未実施は「—」。** 空文字だと行の高さが揃わない
        #expect(dips.lastLabel == "—")
    }

    @Test("目標の表示文を作れる")
    func targetLabel() throws {
        let got = try JSONDecoder().decode(TodayRoutine.self, from: Data(today.utf8))

        #expect(got.today?.items.first?.targetLabel == "5セット 6-10")
        // レップ指定が無ければセット数だけ
        #expect(got.today?.items.last?.targetLabel == "4セット")
    }
}

@Suite("今日のルーティンを引く")
struct TodayRoutineAPITests {
    @Test("GET /v1/routines/today を叩く")
    func fetches() async throws {
        let t = FakeTransport(json: today)
        let api = APIClient(baseURL: base, transport: t)

        let got = try await api.todayRoutine(date: "2026-09-28")

        #expect(got.today?.templateName == "胸")
        let url = try #require(t.requests.first?.url)
        #expect(url.path == "/v1/routines/today")
        #expect(url.query?.contains("date=2026-09-28") == true)
    }
}

// ---- 記録したときに Day が残るか（#232）----

private let twoDays = #"""
{"date":"2026-09-28","routineName":"6日サイクル","todayOrder":1,"days":[
  {"dayOrder":1,"templateId":"aaaaaaaa-0000-0000-0000-000000000001","templateName":"胸",
   "items":[{"exerciseId":"22222222-2222-2222-2222-222222222222","exerciseName":"ベンチプレス",
     "muscleGroup":"胸","order":1,"targetSets":5}]},
  {"dayOrder":2,"templateId":"aaaaaaaa-0000-0000-0000-000000000002","templateName":"脚",
   "items":[{"exerciseId":"44444444-4444-4444-4444-444444444444","exerciseName":"スクワット",
     "muscleGroup":"大腿四頭","order":1,"targetSets":4}]}]}
"""#

@Suite("記録したときに Day を残す")
@MainActor
struct LogRoutineTests {
    private func loaded() async -> (LogModel, FakeTransport) {
        let t = FakeTransport()
        t.responses = [
            (Data(#"{"items":[]}"#.utf8), 200),   // listExercises
            (Data(#"{"items":[]}"#.utf8), 200),   // listSessions
            (Data(twoDays.utf8), 200),            // todayRoutine
            (Data(#"{"id":"55555555-5555-5555-5555-555555555555","date":"2026-09-28"}"#.utf8), 201),
        ]
        let m = LogModel(api: APIClient(baseURL: base, transport: t),
                         queue: PendingQueue(store: MemoryPendingStore()),
                         date: "2026-09-28")
        await m.load()

        return (m, t)
    }

    @Test("今日の Day が出る")
    func showsToday() async {
        let (m, _) = await loaded()

        #expect(m.currentDay?.templateName == "胸")
        #expect(m.dayLabel == "1 / 2日目  胸")
    }

    @Test("**手でずらせる**")
    func picksDay() async {
        let (m, _) = await loaded()

        m.pickDay(m.allDays.first { $0.dayOrder == 2 })

        #expect(m.currentDay?.templateName == "脚")
    }

    @Test("自動に戻せる")
    func resetsDay() async {
        let (m, _) = await loaded()
        m.pickDay(m.allDays.last)

        m.pickDay(nil)

        #expect(m.currentDay?.templateName == "胸")
    }

    @Test("ルーティンが無ければ nil")
    func noRoutine() async {
        let t = FakeTransport()
        t.responses = [
            (Data(#"{"items":[]}"#.utf8), 200),
            (Data(#"{"items":[]}"#.utf8), 200),
            (Data(#"{"date":"2026-09-28"}"#.utf8), 200),
        ]
        let m = LogModel(api: APIClient(baseURL: base, transport: t),
                         queue: PendingQueue(store: MemoryPendingStore()),
                         date: "2026-09-28")
        await m.load()

        // **記録はできる。** 画面は今までどおり全種目から選ばせる
        #expect(m.currentDay == nil)
        #expect(m.dayLabel == nil)
    }
}

// ---- ルーティンに無い種目を足す（#232 のバグ）----

@Suite("その日だけ種目を足す")
@MainActor
struct ExtraExerciseTests {
    private func loaded() async -> LogModel {
        let t = FakeTransport()
        t.responses = [
            (Data(#"""
            {"items":[
              {"id":"22222222-2222-2222-2222-222222222222","name":"ベンチプレス","muscleGroup":"胸"},
              {"id":"99999999-9999-9999-9999-999999999999","name":"ラットプルダウン","muscleGroup":"広背筋"}]}
            """#.utf8), 200),                     // listExercises
            (Data(#"{"items":[]}"#.utf8), 200),   // listSessions
            (Data(twoDays.utf8), 200),            // todayRoutine
        ]
        let m = LogModel(api: APIClient(baseURL: base, transport: t),
                         queue: PendingQueue(store: MemoryPendingStore()),
                         date: "2026-09-28")
        await m.load()

        return m
    }

    private let lat = UUID(uuidString: "99999999-9999-9999-9999-999999999999")!
    private let bench = UUID(uuidString: "22222222-2222-2222-2222-222222222222")!

    @Test("**ルーティンに無い種目が行として増える**")
    func addsRow() async {
        let m = await loaded()

        m.addExercise(lat)

        #expect(m.extraExercises.map(\.id) == [lat])
        // 足したらそのまま開く。もう一度探させない
        #expect(m.selectedExerciseId == lat)
    }

    @Test("ルーティンに既にある種目は増やさない")
    func skipsExisting() async {
        let m = await loaded()

        m.addExercise(bench)

        #expect(m.extraExercises.isEmpty)
        // 行は既にあるので、開くだけ
        #expect(m.selectedExerciseId == bench)
    }

    @Test("二重に足さない")
    func noDuplicate() async {
        let m = await loaded()

        m.addExercise(lat)
        m.addExercise(lat)

        #expect(m.extraExercises.count == 1)
    }

    @Test("**日を移ったら消える**")
    func clearsOnDateChange() async {
        let m = await loaded()
        m.addExercise(lat)

        await m.goToPreviousDay()

        #expect(m.extraExercises.isEmpty)
    }
}
