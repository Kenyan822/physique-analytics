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
    private(set) var recording = false
    private(set) var pendingMessage = ""

    var selectedExerciseId: UUID?

    /// **数値は文字列のまま持つ**（食事と同じ理由・#229）。
    /// 数値に直しながら持つと、最後の1桁を消したときに戻る
    var weightText = "20"
    var repsText = "8"
    var rir: Int? = 2

    /// 送るときの値。**読めなければ 0**
    var weightKg: Double { Double(weightText.trimmingCharacters(in: .whitespaces)) ?? 0 }
    var reps: Int { Int(repsText.trimmingCharacters(in: .whitespaces)) ?? 0 }

    /// ± ボタン。**打つのと両方できるようにする**
    func bumpWeight(_ delta: Double) {
        weightText = numberText(max(0, weightKg + delta))
    }

    func bumpReps(_ delta: Int) {
        repsText = String(max(0, reps + delta))
    }

    /// 種目を開く／閉じる。**同じ行を押したら閉じる**（#232）
    func toggleExercise(_ id: UUID) {
        selectedExerciseId = selectedExerciseId == id ? nil : id
    }

    /// その日だけ足した種目（#232）。ルーティンには入らない
    private(set) var extraExerciseIds: [UUID] = []

    /// 足した種目を行として出すための実体。**並びは足した順**
    var extraExercises: [Exercise] {
        extraExerciseIds.compactMap { id in exercises.first { $0.id == id } }
    }

    /// 今日の想定に無い種目を足す（#232）。
    ///
    /// **行を増やしてから開く。** 開くだけだと入力欄はルーティンの行の下にしか
    /// 出ないので、足した種目を選んでも画面に何も起きない（実機で踏んだ）。
    ///
    /// 永続化はまだしない。セッションに持たせるのは #242。
    func addExercise(_ id: UUID) {
        defer { selectedExerciseId = id }

        // 既に行があるなら増やさない。開くだけでよい
        guard currentDay?.items.contains(where: { $0.exerciseId == id }) != true,
              !extraExerciseIds.contains(id) else { return }

        extraExerciseIds.append(id)
    }

    var showError = false
    private(set) var errorMessage = ""

    /// 表示している日。**前日・翌日に移動できる**（食事と同じ形・#232）
    private(set) var date: String
    /// 「今日」。これより先には進めない。テストで固定するため引数にする
    private let today: String
    private let api: APIClient
    private let queue: PendingQueue
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
        extraExerciseIds = []
        await load()
    }

    // MARK: - 読み込み

    func load() async {
        do {
            exercises = try await api.listExercises()
            // その日の記録を先に読む。画面を開き直したときにセット番号が
            // 1 に戻ると、既に記録した番号と衝突して入力できない
            let sessions = try await api.listSessions(from: date, to: date, limit: 1)
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
            let res = try await api.lastPerformance(exerciseId: id)
            last = res
            // **前回値をそのまま初期値にする（要件 T-02）。**
            // ジムでの入力の大半は「前回と同じか少し増やす」
            if let top = res.sets?.first {
                let w = (top.weightKg / Self.weightStep).rounded() * Self.weightStep
                weightText = numberText(w)
                repsText = String(top.reps)
                rir = top.rir
            }
        } catch {
            report(error)
        }
    }

    // MARK: - 記録

    /// 次のセット番号。**件数ではなく最大値 + 1**。
    /// 途中のセットを消したあとに件数で決めると、既存の番号と衝突する
    var nextSetNo: Int {
        guard let id = selectedExerciseId else { return 1 }

        return (logged.filter { $0.exerciseId == id }.map(\.setNo).max() ?? 0) + 1
    }

    func record() async {
        guard let exerciseId = selectedExerciseId else { return }
        recording = true
        defer { recording = false }

        let item = PendingSet(
            id: UUID(), date: date, exerciseId: exerciseId, setNo: nextSetNo,
            weightKg: weightKg, reps: reps, rir: rir, queuedAt: Date()
        )

        // **積んだ時点で記録は確定。** 送信の成否は表示で伝えるだけにする。
        // 「失敗したら入力し直し」では電波の悪いジムで使えない
        queue.push(item)
        logged.append(item)
        startRest(for: exerciseId)

        await flush()
        updatePendingMessage()
    }

    /// 溜まった記録を送る（要件 T-07）。
    func flush() async {
        var sent: [UUID] = []

        for item in queue.all() {
            do {
                let session = try await ensureSession()
                _ = try await api.createSet(
                    sessionId: session.id,
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

    private func ensureSession() async throws -> WorkoutSession {
        let existing = try await api.listSessions(from: date, to: date, limit: 1)
        if let s = existing.first { return s }

        // **どの Day をやったかを残す。** 入れないと巡回が進まない（#232）
        return try await api.createSession(
            WorkoutSessionInput(date: date, templateId: currentDay?.templateId)
        )
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
