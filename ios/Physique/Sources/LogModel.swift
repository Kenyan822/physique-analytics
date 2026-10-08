import Foundation
import Observation

/// 入力画面の状態。
///
/// UI から切り離してあるのは、**ロジックを `swift test` で回せるようにする**ため
/// （Xcode を開かずにテストできる）。
@Observable
@MainActor
final class LogModel {
    /// 重量の刻み（要件 T-03）。プレートの最小単位が 1.25kg × 2 なので 2.5kg
    static let weightStep: Double = 2.5

    /// 休憩の既定値。多関節は回復に時間が要る
    static let compoundRestSec = 180
    static let isolationRestSec = 90

    private(set) var exercises: [Exercise] = []

    // MARK: - 今日やる想定（要件 T-01 / #232）

    /// **nil ならルーティン未登録。** そのときも記録はできる
    private(set) var routine: TodayRoutine?
    /// 手でずらした Day の `dayOrder`。nil ならサーバの判断に従う
    private(set) var pickedOrder: Int?

    /// 画面に出す Day。手でずらしていればそちらを優先する
    var currentDay: RoutineDay? {
        guard let t = routine else { return nil }
        guard let order = pickedOrder else { return t.today }

        return t.days?.first { $0.dayOrder == order }
    }

    /// 手でずらすときの選択肢
    var allDays: [RoutineDay] { routine?.days ?? [] }

    /// 「1 / 6日目 胸」。ルーティンが無ければ nil
    var dayLabel: String? {
        guard let d = currentDay else { return nil }
        let total = routine?.totalDays ?? 0
        guard total > 0 else { return d.templateName }

        return "\(d.dayOrder) / \(total)日目  \(d.templateName)"
    }

    private(set) var logged: [PendingSet] = []
    private(set) var last: LastPerformance?
    private(set) var loadingLast = false
    private(set) var pendingMessage = ""

    var selectedExerciseId: UUID? {
        didSet {
            guard selectedExerciseId != oldValue else { return }
            drafts = []
            rebuildDrafts()
        }
    }

    /// **数値は文字列のまま持つ**（食事と同じ理由・#229）。
    /// 数値に直しながら持つと、最後の1桁を消したときに戻る
    /// 開いている種目のセット（#256）。**記録済みも入力中も同じ行。**
    ///
    /// 確定ボタンを置かず、欄から離れた時点で保存する。ジムで「記録を押し忘れて
    /// 1セット消えた」が起きないようにする。
    struct SetDraft: Identifiable, Hashable {
        /// クライアント生成の UUID。そのままセットの id になる（ADR-0014）
        let id: UUID
        var setNo: Int
        var weightText: String
        var repsText: String
        var rirText: String
        /// サーバに届いたか。false ならまだ送っていない行
        var recorded: Bool

        var weightKg: Double? { Double(weightText.trimmingCharacters(in: .whitespaces)) }
        var reps: Int? { Int(repsText.trimmingCharacters(in: .whitespaces)) }
        var rir: Int? { Int(rirText.trimmingCharacters(in: .whitespaces)) }

        /// **空欄のまま保存しない。** 0kg × 0回 が残ると分析が狂う
        var isComplete: Bool { weightKg != nil && reps != nil }
    }

    private(set) var drafts: [SetDraft] = []

    func setWeight(_ v: String, for id: UUID) { update(id) { $0.weightText = v } }
    func setReps(_ v: String, for id: UUID) { update(id) { $0.repsText = v } }
    func setRir(_ v: String, for id: UUID) { update(id) { $0.rirText = v } }

    private func update(_ id: UUID, _ change: (inout SetDraft) -> Void) {
        guard let i = drafts.firstIndex(where: { $0.id == id }) else { return }
        change(&drafts[i])
    }

    /// 記録済みから行を組み直す。**打ちかけの行は残す**
    func rebuildDrafts() {
        guard let ex = selectedExerciseId else {
            drafts = []

            return
        }

        let unsaved = drafts.filter { !$0.recorded }
        drafts = sets(of: ex).map { s in
            SetDraft(id: s.id, setNo: s.setNo,
                     weightText: numberText(s.weightKg), repsText: String(s.reps),
                     rirText: s.rir.map(String.init) ?? "", recorded: true)
        } + unsaved

        // **常に打てる行を1つ用意する。** 記録済みだけだと打つ場所が無い（実機で踏んだ）
        if !drafts.contains(where: { !$0.recorded }) { addSetRow() }
    }

    /// セットを1行足す。**直前の値を引き継ぐ。** 大半は「前と同じか少し増やす」
    func addSetRow() {
        let prev = drafts.last
        drafts.append(SetDraft(
            id: UUID(), setNo: (drafts.map(\.setNo).max() ?? 0) + 1,
            weightText: prev?.weightText ?? "", repsText: prev?.repsText ?? "",
            rirText: prev?.rirText ?? "", recorded: false))
    }

    /// 行を確定する。**欄から離れたときに呼ぶ。**
    ///
    /// 未記録なら記録、記録済みで値が変わっていれば直す。
    /// **変わっていなければ何も送らない**（欄を移動するたびに往復しない）
    func commitDraft(_ id: UUID) async {
        guard let i = drafts.firstIndex(where: { $0.id == id }),
              let exerciseId = selectedExerciseId else { return }
        let d = drafts[i]
        guard d.isComplete, let w = d.weightKg, let r = d.reps else { return }

        if !d.recorded {
            let item = PendingSet(
                id: d.id, date: date, exerciseId: exerciseId, setNo: d.setNo,
                weightKg: w, reps: r, rir: d.rir, queuedAt: Date())

            // **積んだ時点で記録は確定。** 送信の成否は表示で伝えるだけにする
            queue.push(item)
            logged.append(item)
            drafts[i].recorded = true
            startRest(for: exerciseId)

            await flush()
            updatePendingMessage()

            return
        }

        guard let saved = logged.first(where: { $0.id == id }),
              saved.weightKg != w || saved.reps != r || saved.rir != d.rir else { return }

        await updateSet(id: id, weightKg: w, reps: r, rir: d.rir)
    }

    /// 行を消す。
    ///
    /// **まだ送っていない行はサーバを叩かない。** 叩くと 404 になる（実機で踏んだ）
    func deleteDraft(_ id: UUID) async {
        guard let d = drafts.first(where: { $0.id == id }) else { return }

        if !d.recorded {
            drafts.removeAll { $0.id == id }
            rebuildDrafts()

            return
        }

        // 送信待ちならキューから抜くだけでよい
        if queue.all().contains(where: { $0.id == id }) {
            queue.remove(ids: [id])
            logged.removeAll { $0.id == id }
            drafts.removeAll { $0.id == id }
            rebuildDrafts()
            updatePendingMessage()

            return
        }

        await deleteSet(id: id)
        drafts.removeAll { $0.id == id }
        rebuildDrafts()
    }

    /// 「10/1(木)  80×8 / 80×7」。**前回この種目をやったセッションの全セット**。
    /// 直前の1セットではない（それは今日の記録として上に並んでいる）
    var lastSummary: String? {
        guard let date = last?.date, let sets = last?.sets, !sets.isEmpty else { return nil }

        let body = sets
            .sorted { $0.setNo < $1.setNo }
            .map { "\(numberText($0.weightKg))×\($0.reps)" }
            .joined(separator: " / ")

        return "\(JST.displayString(from: date))  \(body)"
    }

    /// 種目を開く／閉じる。**同じ行を押したら閉じる**（#232）
    func toggleExercise(_ id: UUID) {
        selectedExerciseId = selectedExerciseId == id ? nil : id
    }

    // MARK: - その日の種目リスト（#242 / #247）

    /// 画面に並べる1行。ルーティン由来でも手で足したものでも同じ形にする
    struct Row: Identifiable, Hashable {
        var id: UUID { exerciseId }

        let exerciseId: UUID
        let name: String
        let muscleGroup: MuscleGroup
        /// ルーティンの目標。手で足した種目には無い
        var target: String?
    }

    /// サーバに保存されたその日の並び。**空ならまだ触っていない**
    private(set) var savedOrder: [UUID] = []
    /// 画面に並べる行。保存された並びがあればそちら、無ければルーティン
    private(set) var rows: [Row] = []
    private(set) var savingRows = false

    /// 行を組み直す。**保存された並びが正、無ければルーティン**（#242）
    private func rebuildRows() {
        let fromRoutine = (currentDay?.items ?? []).map {
            Row(exerciseId: $0.exerciseId, name: $0.exerciseName,
                muscleGroup: MuscleGroup(rawValue: $0.muscleGroup) ?? .chest,
                target: $0.targetLabel)
        }
        guard !savedOrder.isEmpty else {
            rows = fromRoutine

            return
        }

        let byId = Dictionary(uniqueKeysWithValues: fromRoutine.map { ($0.exerciseId, $0) })
        rows = savedOrder.compactMap { id in
            if let r = byId[id] { return r }
            guard let e = exercises.first(where: { $0.id == id }) else { return nil }

            return Row(exerciseId: e.id, name: e.name, muscleGroup: e.muscleGroup)
        }
    }

    /// 並べ替える。**先に画面を動かしてから保存する**（ドラッグが固まらないように）
    func moveRows(from source: IndexSet, to destination: Int) async {
        let before = rows
        rows = Self.moved(rows, from: source, to: destination)
        await persistRows(revertTo: before)
    }

    /// 行を消す。**セットのある種目はサーバが 422 で弾く**（#242）
    func removeRows(_ offsets: IndexSet) async {
        let before = rows
        rows = rows.enumerated().filter { !offsets.contains($0.offset) }.map(\.element)
        await persistRows(revertTo: before)
    }

    /// 今日の想定に無い種目を足す。**末尾に入る**
    func addExercise(_ id: UUID) async {
        defer { selectedExerciseId = id }
        guard !rows.contains(where: { $0.exerciseId == id }) else { return }
        guard let e = exercises.first(where: { $0.id == id }) else { return }

        let before = rows
        rows.append(Row(exerciseId: e.id, name: e.name, muscleGroup: e.muscleGroup))
        await persistRows(revertTo: before)
    }

    /// `move(fromOffsets:toOffset:)` の中身。**SwiftUI の拡張なので自前で書く**
    /// （`PhysiqueCore` は SwiftUI に依存しない。そうしないと `swift test` で回せない）
    static func moved(_ list: [Row], from source: IndexSet, to destination: Int) -> [Row] {
        let picked = source.sorted().map { list[$0] }
        var rest = list
        for i in source.sorted(by: >) { rest.remove(at: i) }
        // 抜いた分だけ挿入位置が前にずれる
        let at = destination - source.filter { $0 < destination }.count

        rest.insert(contentsOf: picked, at: min(max(0, at), rest.count))

        return rest
    }

    /// 並びをサーバに全置換で送る。
    ///
    /// **失敗したら画面も戻す。** 消えたまま残ると、次の全置換で本当に消える
    private func persistRows(revertTo before: [Row]) async {
        savingRows = true
        defer { savingRows = false }

        do {
            let sid = try await ensureSessionID()
            let saved = try await api.replaceSessionExercises(
                sessionId: sid, exerciseIds: rows.map(\.exerciseId))
            savedOrder = saved.isEmpty ? rows.map(\.exerciseId) : saved.map(\.exerciseId)
            rebuildRows()
        } catch {
            rows = before
            savedOrder = before.map(\.exerciseId)
            report(error)
        }
    }

    var showError = false
    private(set) var errorMessage = ""

    /// 表示している日。**前日・翌日に移動できる**（食事と同じ形・#232）
    private(set) var date: String
    /// 「今日」。これより先には進めない。テストで固定するため引数にする
    private let today: String
    private let api: APIClient
    private let queue: PendingQueue
    /// 読み込み済みのセッション id。日を移ったら捨てる
    private var knownSessionID: UUID?
    private var restStartedAt: Date?
    private var restSeconds = 0

    init(
        api: APIClient = APIClient(baseURL: AppConfig.apiBaseURL),
        queue: PendingQueue = PendingQueue(store: FilePendingStore()),
        date: String = JST.dateString(),
        today: String = JST.dateString()
    ) {
        self.api = api
        self.queue = queue
        self.date = date
        self.today = today
    }

    // MARK: - 日付の行き来（食事と同じ形・#232）

    /// `9/28(月)` の形
    var dateLabel: String { JST.displayString(from: date) }

    /// **今日より先には進めない。** 記録できない日を開いても意味が無い
    var canGoNext: Bool { date < today }

    func goToPreviousDay() async {
        await move(to: JST.shift(date, days: -1))
    }

    func goToNextDay() async {
        guard canGoNext else { return }
        await move(to: JST.shift(date, days: 1))
    }

    /// 日付を選び直す。未来を選んだら今日に丸める
    func goTo(_ newDate: String) async {
        await move(to: min(newDate, today))
    }

    private func move(to newDate: String) async {
        guard newDate != date else { return }
        date = newDate
        // **手でずらした Day も足した種目も持ち越さない。**
        // 別の日の選択が残ると混乱する
        pickedOrder = nil
        selectedExerciseId = nil
        savedOrder = []
        rows = []
        knownSessionID = nil
        await load()
    }

    // MARK: - 読み込み

    func load() async {
        do {
            exercises = try await api.listExercises()
            // その日の記録を先に読む。画面を開き直したときにセット番号が
            // 1 に戻ると、既に記録した番号と衝突して入力できない
            let sessions = try await api.listSessions(from: date, to: date, limit: 1)
            knownSessionID = sessions.first?.id
            savedOrder = (sessions.first?.exercises ?? [])
                .sorted { $0.itemOrder < $1.itemOrder }
                .map(\.exerciseId)
            logged = (sessions.first?.sets ?? []).map {
                PendingSet(
                    id: $0.id, date: date, exerciseId: $0.exerciseId, setNo: $0.setNo,
                    weightKg: $0.weightKg, reps: $0.reps, rir: $0.rir, queuedAt: Date()
                )
            }
            await flush()
        } catch {
            // 読めなくても入力はできる。積んでおけば復帰時に送られる
            report(error)
        }

        // **失敗しても握る。** 今日の想定が出ないだけで、記録はできる
        routine = try? await api.todayRoutine(date: date)

        rebuildRows()
        rebuildDrafts()
        updatePendingMessage()
    }

    /// Day を手でずらす（要件 T-01 / #232）。
    ///
    /// **サーバの判断を上書きするだけ。** 記録すればその Day が履歴に残り、
    /// 次回からはそこを起点に巡回する。
    ///
    /// nil で自動に戻す。
    /// nil で自動に戻す。**全 Day を持っているので引き直さない。**
    func pickDay(_ day: RoutineDay?) {
        pickedOrder = day?.dayOrder
        rebuildRows()
    }

    /// その種目を今日やったか。行の印に使う
    func isDoneToday(_ exerciseId: UUID) -> Bool {
        logged.contains { $0.exerciseId == exerciseId }
    }

    func selectExercise(_ id: UUID?) async {
        last = nil
        restStartedAt = nil
        guard let id else { return }

        loadingLast = true
        defer { loadingLast = false }

        do {
            // **表示日より前**で引く。今日を含めると、今打った値が「前回」になる（#257）
            let res = try await api.lastPerformance(exerciseId: id, before: date)
            last = res
            // **前回値をそのまま初期値にする（要件 T-02）。**
            // ジムでの入力の大半は「前回と同じか少し増やす」。
            // 打ちかけが1行も無いときだけ（打った値を上書きしない）
            // **まだ何も打っていない行にだけ入れる。** 打った値を上書きしない
            if let top = res.sets?.first,
               let i = drafts.firstIndex(where: { !$0.recorded && $0.weightText.isEmpty }) {
                let w = (top.weightKg / Self.weightStep).rounded() * Self.weightStep
                drafts[i].weightText = numberText(w)
                drafts[i].repsText = String(top.reps)
                drafts[i].rirText = top.rir.map(String.init) ?? ""
            }
        } catch {
            report(error)
        }
    }

    // MARK: - 記録



    /// 溜まった記録を送る（要件 T-07）。
    // MARK: - 記録したセットを直す・消す（#247）

    /// その種目のセット。**番号順**
    func sets(of exerciseId: UUID) -> [PendingSet] {
        logged.filter { $0.exerciseId == exerciseId }.sorted { $0.setNo < $1.setNo }
    }

    /// 打ち間違いを直す。
    ///
    /// **キューを経由しない。** 未送信のものを直す経路まで作ると、同じ id が
    /// キューとサーバの両方にある状態を考えることになる。直せるのは送信済みだけ
    func updateSet(id: UUID, weightKg: Double, reps: Int, rir: Int?) async {
        guard let i = logged.firstIndex(where: { $0.id == id }) else { return }

        do {
            _ = try await api.updateSet(id: id, WorkoutSetInput(
                id: id, exerciseId: logged[i].exerciseId, setNo: logged[i].setNo,
                weightKg: weightKg, reps: reps, rir: rir))
            logged[i].weightKg = weightKg
            logged[i].reps = reps
            logged[i].rir = rir
        } catch {
            report(error)
        }
    }

    /// セットを消す。**サーバが消えたことを返すまで画面からは消さない**
    func deleteSet(id: UUID) async {
        do {
            try await api.deleteSet(id: id)
            logged.removeAll { $0.id == id }
        } catch {
            report(error)
        }
    }

    func flush() async {
        var sent: [UUID] = []

        for item in queue.all() {
            do {
                let sid = try await ensureSessionID()
                _ = try await api.createSet(
                    sessionId: sid,
                    WorkoutSetInput(
                        id: item.id, exerciseId: item.exerciseId, setNo: item.setNo,
                        weightKg: item.weightKg, reps: item.reps, rir: item.rir
                    )
                )
                sent.append(item.id)
            } catch let e as APIError where e.isAlreadyRecorded {
                // 送信は届いたが応答が失われた。失敗として扱うと永久に送り直す
                sent.append(item.id)
            } catch {
                // 送れなかったものは残す。消すと記録が消える
                break
            }
        }

        queue.remove(ids: sent)
    }

    /// その日のセッション id。**読み込み済みなら引き直さない。**
    /// セットを記録するたびに一覧を叩くと、ジムの電波で毎回待たされる
    private func ensureSessionID() async throws -> UUID {
        if let id = knownSessionID { return id }

        let existing = try await api.listSessions(from: date, to: date, limit: 1)
        if let s = existing.first {
            knownSessionID = s.id

            return s.id
        }

        // **どの Day をやったかを残す。** 入れないと巡回が進まない（#232）
        let created = try await api.createSession(
            WorkoutSessionInput(date: date, templateId: currentDay?.templateId)
        )
        knownSessionID = created.id

        return created.id
    }

    // MARK: - インターバル（要件 T-05）

    private func startRest(for exerciseId: UUID) {
        let e = exercises.first { $0.id == exerciseId }
        restSeconds = e?.defaultRestSec
            ?? ((e?.isCompound ?? false) ? Self.compoundRestSec : Self.isolationRestSec)
        restStartedAt = restSeconds > 0 ? Date() : nil
    }

    /// 残り時間。**開始時刻からの差で出す。** カウントダウンを持つと、
    /// 画面を消している間に止まってずれる
    var restRemaining: String? {
        guard let started = restStartedAt, restSeconds > 0 else { return nil }

        let left = max(0, Double(restSeconds) - Date().timeIntervalSince(started))
        let s = Int(left.rounded())

        return String(format: "%d:%02d", s / 60, s % 60)
    }

    // MARK: - 表示

    func doneMark(_ exerciseId: UUID) -> String {
        let n = logged.filter { $0.exerciseId == exerciseId }.count

        return n > 0 ? " ✓\(n)" : ""
    }

    func describe(_ s: PendingSet) -> String {
        let name = exercises.first { $0.id == s.exerciseId }?.name ?? "?"
        let rirText = s.rir.map { " @RIR\($0)" } ?? ""

        return "\(name) \(s.setNo)セット目 \(format(s.weightKg))kg × \(s.reps)回\(rirText)"
    }

    func describeLastSets() -> String {
        (last?.sets ?? [])
            .map { "\(format($0.weightKg))×\($0.reps)" + ($0.rir.map { "@\($0)" } ?? "") }
            .joined(separator: "  ")
    }

    private func format(_ v: Double) -> String {
        v == v.rounded() ? String(Int(v)) : String(v)
    }

    private func updatePendingMessage() {
        let n = queue.count
        pendingMessage = n > 0 ? "未送信 \(n) 件。オンライン復帰時に送る" : ""
    }

    private func report(_ error: Error) {
        errorMessage = (error as? APIError)?.errorDescription ?? error.localizedDescription
        showError = true
    }
}

/// 接続先と鍵。すべて `Info.plist` から読む。
///
/// **実機では localhost に届かない。** 未設定のまま実機に入れると、
/// 何をしても通信できない画面になる。ビルド設定で必ず入れる。
enum AppConfig {
    static var apiBaseURL: URL {
        url(for: "API_BASE_URL") ?? URL(string: "http://localhost:8080")!
    }

    /// Supabase のプロジェクト URL。ログインに使う
    static var supabaseURL: URL? { url(for: "SUPABASE_URL") }

    /// 公開前提の鍵。単体では何も読めない（DB は RLS で閉じている）
    static var supabaseAnonKey: String? { string(for: "SUPABASE_ANON_KEY") }

    private static func string(for key: String) -> String? {
        guard let s = Bundle.main.object(forInfoDictionaryKey: key) as? String,
              !s.isEmpty else { return nil }

        return s
    }

    private static func url(for key: String) -> URL? {
        string(for: key).flatMap(URL.init(string:))
    }
}
