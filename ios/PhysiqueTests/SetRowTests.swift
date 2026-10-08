import Foundation
import Testing

@testable import PhysiqueCore

private let base = URL(string: "http://api.test")!

private let bench = UUID(uuidString: "22222222-2222-2222-2222-222222222222")!
private let dips = UUID(uuidString: "33333333-3333-3333-3333-333333333333")!
private let lat = UUID(uuidString: "99999999-9999-9999-9999-999999999999")!
private let sessionId = UUID(uuidString: "55555555-5555-5555-5555-555555555555")!

private let exercises = #"""
{"items":[
  {"id":"22222222-2222-2222-2222-222222222222","name":"ベンチプレス","muscleGroup":"胸"},
  {"id":"33333333-3333-3333-3333-333333333333","name":"ディップス","muscleGroup":"胸"},
  {"id":"99999999-9999-9999-9999-999999999999","name":"ラットプルダウン","muscleGroup":"広背筋"}]}
"""#

private let routine = #"""
{"date":"2026-10-08","routineName":"6日サイクル","todayOrder":1,"days":[
  {"dayOrder":1,"templateId":"aaaaaaaa-0000-0000-0000-000000000001","templateName":"胸","items":[
    {"exerciseId":"22222222-2222-2222-2222-222222222222","exerciseName":"ベンチプレス",
     "muscleGroup":"胸","order":1,"targetSets":5},
    {"exerciseId":"33333333-3333-3333-3333-333333333333","exerciseName":"ディップス",
     "muscleGroup":"胸","order":2,"targetSets":4}]}]}
"""#

/// 種目リストを持っているセッション（並べ替え済み）
private let sessionWithList = #"""
{"items":[{"id":"55555555-5555-5555-5555-555555555555","date":"2026-10-08","sets":[],
 "exercises":[
   {"exerciseId":"33333333-3333-3333-3333-333333333333","exerciseName":"ディップス",
    "muscleGroup":"胸","itemOrder":1},
   {"exerciseId":"22222222-2222-2222-2222-222222222222","exerciseName":"ベンチプレス",
    "muscleGroup":"胸","itemOrder":2}]}]}
"""#

private let emptySession = #"""
{"items":[{"id":"55555555-5555-5555-5555-555555555555","date":"2026-10-08","sets":[],"exercises":[]}]}
"""#

@MainActor
private func model(_ sessions: String) async -> (LogModel, FakeTransport) {
    let t = FakeTransport()
    t.responses = [
        (Data(exercises.utf8), 200),
        (Data(sessions.utf8), 200),
        (Data(routine.utf8), 200),
    ]
    let m = LogModel(api: APIClient(baseURL: base, transport: t),
                     queue: PendingQueue(store: MemoryPendingStore()),
                     date: "2026-10-08", today: "2026-10-08")
    await m.load()

    return (m, t)
}

@Suite("その日の種目リスト")
@MainActor
struct SessionExerciseListTests {
    @Test("**リストが空ならルーティンの並びを使う**")
    func fallsBackToRoutine() async {
        let (m, _) = await model(emptySession)

        #expect(m.rows.map(\.exerciseId) == [bench, dips])
    }

    @Test("**リストがあればそちらが正**（ルーティンより優先）")
    func sessionListWins() async {
        let (m, _) = await model(sessionWithList)

        #expect(m.rows.map(\.exerciseId) == [dips, bench])
    }

    @Test("並べ替えるとサーバに全置換で送る")
    func reorderSaves() async {
        let (m, t) = await model(emptySession)
        t.responses = [(Data(#"{"items":[]}"#.utf8), 200)]

        await m.moveRows(from: IndexSet(integer: 1), to: 0)

        #expect(m.rows.map(\.exerciseId) == [dips, bench])
        let req = try! #require(t.requests.last)
        #expect(req.httpMethod == "PUT")
        #expect(req.url?.path == "/v1/workout-sessions/\(sessionId.uuidString.lowercased())/exercises")
        let body = try! JSONSerialization.jsonObject(with: req.httpBody!) as! [String: Any]
        #expect(body["exerciseIds"] as? [String]
                == [dips.uuidString.lowercased(), bench.uuidString.lowercased()])
    }

    @Test("種目を足すと末尾に入る")
    func addsToEnd() async {
        let (m, t) = await model(emptySession)
        t.responses = [(Data(#"{"items":[]}"#.utf8), 200)]

        await m.addExercise(lat)

        #expect(m.rows.map(\.exerciseId) == [bench, dips, lat])
    }

    @Test("既にある種目は二重に足さない")
    func noDuplicate() async {
        let (m, t) = await model(emptySession)
        t.responses = [(Data(#"{"items":[]}"#.utf8), 200)]

        await m.addExercise(bench)

        #expect(m.rows.map(\.exerciseId) == [bench, dips])
    }

    @Test("種目を消せる")
    func removes() async {
        let (m, t) = await model(emptySession)
        t.responses = [(Data(#"{"items":[]}"#.utf8), 200)]

        await m.removeRows(IndexSet(integer: 0))

        #expect(m.rows.map(\.exerciseId) == [dips])
    }

    @Test("**日を移ると持ち越さない**")
    func clearsOnDateChange() async {
        let (m, t) = await model(sessionWithList)
        #expect(m.rows.map(\.exerciseId) == [dips, bench])

        // 前日はまだ触っていない日＝ルーティンの並び
        t.responses = [
            (Data(exercises.utf8), 200),
            (Data(#"{"items":[]}"#.utf8), 200),
            (Data(routine.utf8), 200),
        ]
        await m.goToPreviousDay()

        #expect(m.rows.map(\.exerciseId) == [bench, dips])
    }

    @Test("**サーバが拒否したら画面も元に戻す**")
    func revertsOnFailure() async {
        let (m, t) = await model(emptySession)
        // セットのある種目を外そうとした、など（#242 の 422）
        let rejected = #"{"type":"about:blank","title":"外せない","status":422,"detail":"記録のある種目は外せない"}"#
        t.responses = [(Data(rejected.utf8), 422)]

        await m.removeRows(IndexSet(integer: 0))

        // **戻す。** 消えたまま残ると、次の全置換で本当に消える
        #expect(m.rows.map(\.exerciseId) == [bench, dips])
        #expect(m.showError)
    }
}

/// 記録済みセットを直す・消す（#247）。
///
/// **API は前からある**（`PATCH` / `DELETE /v1/workout-sets/{setId}`）。
/// iOS に無かっただけ。
private let sessionWithSets = #"""
{"items":[{"id":"55555555-5555-5555-5555-555555555555","date":"2026-10-08","exercises":[],
 "sets":[
  {"id":"77777777-0000-0000-0000-000000000001","sessionId":"55555555-5555-5555-5555-555555555555",
   "exerciseId":"22222222-2222-2222-2222-222222222222","setNo":1,"weightKg":80,"reps":8,"rir":2},
  {"id":"77777777-0000-0000-0000-000000000002","sessionId":"55555555-5555-5555-5555-555555555555",
   "exerciseId":"22222222-2222-2222-2222-222222222222","setNo":2,"weightKg":80,"reps":7,"rir":1}]}]}
"""#

@Suite("記録したセットを直す・消す")
@MainActor
struct EditSetTests {
    private let set1 = UUID(uuidString: "77777777-0000-0000-0000-000000000001")!

    @Test("種目ごとのセットが並ぶ")
    func listsSets() async {
        let (m, _) = await model(sessionWithSets)

        #expect(m.sets(of: bench).map(\.setNo) == [1, 2])
        #expect(m.sets(of: dips).isEmpty)
    }

    @Test("**セット番号は件数ではなく最大値 + 1**")
    func setNoFromMax() async {
        let (m, t) = await model(sessionWithSets)
        m.selectedExerciseId = bench
        t.responses = [(Data("{}".utf8), 204)]

        // 1セット目を消しても、次は 3（2 の次）。件数で決めると 2 と衝突する
        await m.deleteDraft(UUID(uuidString: "77777777-0000-0000-0000-000000000001")!)

        // 空行は自動で用意される（3）。さらに足すと 4
        #expect(m.drafts.map(\.setNo) == [2, 3])
        m.addSetRow()
        #expect(m.drafts.map(\.setNo) == [2, 3, 4])
    }

    @Test("直すと PATCH を送る")
    func patches() async {
        let (m, t) = await model(sessionWithSets)
        let patched = #"{"id":"77777777-0000-0000-0000-000000000001","sessionId":"55555555-5555-5555-5555-555555555555","exerciseId":"22222222-2222-2222-2222-222222222222","setNo":1,"weightKg":85,"reps":6,"rir":1}"#
        t.responses = [(Data(patched.utf8), 200)]

        await m.updateSet(id: set1, weightKg: 85, reps: 6, rir: 1)

        let req = try! #require(t.requests.last)
        #expect(req.httpMethod == "PATCH")
        #expect(req.url?.path == "/v1/workout-sets/\(set1.uuidString.lowercased())")
        // 画面も直る
        #expect(m.sets(of: bench).first?.weightKg == 85)
        #expect(m.sets(of: bench).first?.reps == 6)
    }

    @Test("消すと DELETE を送って行が消える")
    func deletes() async {
        let (m, t) = await model(sessionWithSets)
        t.responses = [(Data("{}".utf8), 204)]

        await m.deleteSet(id: set1)

        let req = try! #require(t.requests.last)
        #expect(req.httpMethod == "DELETE")
        #expect(m.sets(of: bench).map(\.setNo) == [2])
    }

    @Test("**失敗したら消さない**")
    func keepsOnFailure() async {
        let (m, t) = await model(sessionWithSets)
        let err = #"{"type":"about:blank","title":"見つからない","status":404}"#
        t.responses = [(Data(err.utf8), 404)]

        await m.deleteSet(id: set1)

        #expect(m.sets(of: bench).count == 2)
        #expect(m.showError)
    }
}

/// 入力を1行に縮める（#251）。
private let withLast = #"""
{"exerciseId":"22222222-2222-2222-2222-222222222222","date":"2026-10-01","estimatedOneRm":102.5,
 "sets":[
  {"id":"88888888-0000-0000-0000-000000000001","sessionId":"66666666-6666-6666-6666-666666666666",
   "exerciseId":"22222222-2222-2222-2222-222222222222","setNo":1,"weightKg":80,"reps":8,"rir":2},
  {"id":"88888888-0000-0000-0000-000000000002","sessionId":"66666666-6666-6666-6666-666666666666",
   "exerciseId":"22222222-2222-2222-2222-222222222222","setNo":2,"weightKg":80,"reps":7,"rir":1}]}
"""#

@Suite("1行の入力")
@MainActor
struct CompactInputTests {
    @Test("RIR も文字列で持つ（空にできる）")
    func rirIsText() async {
        let (m, _) = await model(emptySession)
        m.selectedExerciseId = bench
        m.addSetRow()
        let id = m.drafts.last!.id

        m.setRir("2", for: id)
        #expect(m.drafts.last?.rir == 2)

        // **空にできる。** 数値で持つと最後の1桁が消せない（#229）
        m.setRir("", for: id)
        #expect(m.drafts.last?.rir == nil)
    }

    @Test("読めない RIR は未入力あつかい")
    func badRir() async {
        let (m, _) = await model(emptySession)
        m.selectedExerciseId = bench
        m.addSetRow()
        let id = m.drafts.last!.id

        m.setRir("あ", for: id)

        #expect(m.drafts.last?.rir == nil)
    }

    @Test("**重量かレップが空なら保存しない**")
    func incompleteRow() async {
        let (m, _) = await model(emptySession)
        m.selectedExerciseId = bench
        m.addSetRow()
        let id = m.drafts.last!.id

        m.setWeight("80", for: id); m.setReps("8", for: id)
        #expect(m.drafts.last?.isComplete == true)

        m.setWeight("", for: id)
        #expect(m.drafts.last?.isComplete == false)

        m.setWeight("80", for: id); m.setReps("", for: id)
        #expect(m.drafts.last?.isComplete == false)
    }

    @Test("**前回は日付とそのセッションの全セット**")
    func lastSummary() async {
        let (m, t) = await model(emptySession)
        t.responses = [(Data(withLast.utf8), 200)]

        await m.selectExercise(bench)

        #expect(m.lastSummary == "10/1(木)  80×8 / 80×7")
    }

    @Test("初回なら nil")
    func noLast() async {
        let (m, t) = await model(emptySession)
        t.responses = [(Data(#"{"exerciseId":"22222222-2222-2222-2222-222222222222"}"#.utf8), 200)]

        await m.selectExercise(bench)

        #expect(m.lastSummary == nil)
    }
}

/// 行ごとその場で直す（#256）。
@Suite("行ごとに直す")
@MainActor
struct DraftRowTests {
    private let set1 = UUID(uuidString: "77777777-0000-0000-0000-000000000001")!

    @Test("記録済みが行として出る")
    func drafts() async {
        let (m, _) = await model(sessionWithSets)
        m.selectedExerciseId = bench

        // 記録済み2行 + **常に打てる空行が1つ**
        #expect(m.drafts.map(\.setNo) == [1, 2, 3])
        #expect(m.drafts.first?.weightText == "80")
        #expect(m.drafts.first?.rirText == "2")
        #expect(m.drafts.filter(\.recorded).count == 2)
    }

    @Test("**＋で1行増える。直前の値を引き継ぐ**")
    func addsRow() async {
        let (m, _) = await model(sessionWithSets)
        m.selectedExerciseId = bench

        m.addSetRow()

        #expect(m.drafts.map(\.setNo) == [1, 2, 3, 4])
        let added = m.drafts.last!
        #expect(added.weightText == "80")
        #expect(added.repsText == "7")
        #expect(!added.recorded)
    }

    @Test("打った値を確定すると記録される")
    func commitsNew() async {
        let (m, t) = await model(sessionWithSets)
        m.selectedExerciseId = bench
        m.addSetRow()
        let id = m.drafts.last!.id
        m.setWeight("85", for: id)
        m.setReps("5", for: id)

        let created = #"{"id":"\#(id.uuidString.lowercased())","sessionId":"55555555-5555-5555-5555-555555555555","exerciseId":"22222222-2222-2222-2222-222222222222","setNo":3,"weightKg":85,"reps":5,"rir":1}"#
        t.responses = [(Data(created.utf8), 201)]

        await m.commitDraft(id)

        #expect(m.drafts.last?.recorded == true)
        #expect(m.sets(of: bench).count == 3)
    }

    @Test("**空欄のままでは記録しない**")
    func skipsEmpty() async {
        let (m, t) = await model(sessionWithSets)
        m.selectedExerciseId = bench
        m.addSetRow()
        let id = m.drafts.last!.id
        m.setWeight("", for: id)
        let n = t.requests.count

        await m.commitDraft(id)

        #expect(t.requests.count == n)
        #expect(m.drafts.last?.recorded == false)
    }

    @Test("記録済みを直すと PATCH")
    func commitsExisting() async {
        let (m, t) = await model(sessionWithSets)
        m.selectedExerciseId = bench
        m.setWeight("85", for: set1)
        let patched = #"{"id":"77777777-0000-0000-0000-000000000001","sessionId":"55555555-5555-5555-5555-555555555555","exerciseId":"22222222-2222-2222-2222-222222222222","setNo":1,"weightKg":85,"reps":8,"rir":2}"#
        t.responses = [(Data(patched.utf8), 200)]

        await m.commitDraft(set1)

        #expect(t.requests.last?.httpMethod == "PATCH")
        #expect(m.sets(of: bench).first?.weightKg == 85)
    }

    @Test("変えていなければ送らない")
    func skipsUnchanged() async {
        let (m, t) = await model(sessionWithSets)
        m.selectedExerciseId = bench
        let n = t.requests.count

        await m.commitDraft(set1)

        #expect(t.requests.count == n)
    }

    @Test("**まだ記録していない行は、消してもサーバを叩かない**")
    func deletesUnrecorded() async {
        let (m, t) = await model(sessionWithSets)
        m.selectedExerciseId = bench
        m.addSetRow()
        let id = m.drafts.last!.id
        let n = t.requests.count

        await m.deleteDraft(id)

        #expect(t.requests.count == n)
        #expect(m.drafts.count == 3)
        #expect(!m.showError)
    }

    @Test("記録済みを消すと DELETE")
    func deletesRecorded() async {
        let (m, t) = await model(sessionWithSets)
        m.selectedExerciseId = bench
        t.responses = [(Data("{}".utf8), 204)]

        await m.deleteDraft(set1)

        #expect(t.requests.last?.httpMethod == "DELETE")
        #expect(m.drafts.filter(\.recorded).map(\.setNo) == [2])
    }

    @Test("種目を変えると行も入れ替わる")
    func switchesExercise() async {
        let (m, _) = await model(sessionWithSets)
        m.selectedExerciseId = bench
        #expect(m.drafts.count == 3)

        m.selectedExerciseId = dips

        // 記録は無いが、**打てる空行は1つある**
        #expect(m.drafts.filter(\.recorded).isEmpty)
        #expect(m.drafts.count == 1)
    }
}

/// 「前回」に今日の値が出ていた（#257）。**表示日より前**で引く。
@Suite("前回は表示日より前")
@MainActor
struct LastBeforeTests {
    @Test("**before に表示日を渡す**")
    func passesBefore() async {
        let (m, t) = await model(emptySession)
        t.responses = [(Data(#"{"exerciseId":"22222222-2222-2222-2222-222222222222"}"#.utf8), 200)]

        await m.selectExercise(bench)

        let url = try! #require(t.requests.last?.url)
        #expect(url.path.hasSuffix("/last-performance"))
        // 今日を含めると、今打った値が「前回」として出る
        #expect(url.query?.contains("before=2026-10-08") == true)
    }

    @Test("日を移ったらその日より前で引く")
    func followsDate() async {
        let (m, t) = await model(emptySession)
        t.responses = [
            (Data(exercises.utf8), 200),
            (Data(#"{"items":[]}"#.utf8), 200),
            (Data(routine.utf8), 200),
            (Data(#"{"exerciseId":"22222222-2222-2222-2222-222222222222"}"#.utf8), 200),
        ]
        await m.goToPreviousDay()

        await m.selectExercise(bench)

        let url = try! #require(t.requests.last?.url)
        #expect(url.query?.contains("before=2026-10-07") == true)
    }
}
