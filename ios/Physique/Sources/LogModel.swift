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
    private(set) var logged: [PendingSet] = []
    private(set) var last: LastPerformance?
    private(set) var loadingLast = false
    private(set) var recording = false
    private(set) var pendingMessage = ""

    var selectedExerciseId: UUID?
    var weightKg: Double = 20
    var reps: Int = 8
    var rir: Int? = 2

    var showError = false
    private(set) var errorMessage = ""

    let date: String
    private let api: APIClient
    private let queue: PendingQueue
    private var restStartedAt: Date?
    private var restSeconds = 0

    init(
        api: APIClient = APIClient(baseURL: AppConfig.apiBaseURL),
        queue: PendingQueue = PendingQueue(store: FilePendingStore()),
        date: String = JST.dateString()
    ) {
        self.api = api
        self.queue = queue
        self.date = date
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
        updatePendingMessage()
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
                weightKg = (top.weightKg / Self.weightStep).rounded() * Self.weightStep
                reps = top.reps
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

        return try await api.createSession(WorkoutSessionInput(date: date))
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

/// 接続先。`Info.plist` の `API_BASE_URL` を使い、無ければローカル。
enum AppConfig {
    static var apiBaseURL: URL {
        if let s = Bundle.main.object(forInfoDictionaryKey: "API_BASE_URL") as? String,
           let url = URL(string: s) {
            return url
        }

        // シミュレータからは localhost がそのまま届く
        return URL(string: "http://localhost:8080")!
    }
}
