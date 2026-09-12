import Foundation

/// 送信待ちの記録（要件 T-07）。
///
/// **ジムは電波が悪い。** 送信に失敗した記録をここに溜めて、
/// 復帰時にまとめて送る。
struct PendingSet: Codable, Identifiable, Equatable, Sendable {
    /// クライアントが生成する UUID。再送の冪等性に使う（ADR-0014）
    let id: UUID
    /// JST の日付（ADR-0013）
    let date: String
    let exerciseId: UUID
    let setNo: Int
    let weightKg: Double
    let reps: Int
    var rir: Int?
    /// 積んだ時刻。順序の確認に使う
    let queuedAt: Date
}

/// 保存先。テストで差し替えられるようにする。
protocol PendingStore: Sendable {
    func load() -> Data?
    func save(_ data: Data)
    func clear()
}

/// メモリ上の保存先（テスト用）。
final class MemoryPendingStore: PendingStore, @unchecked Sendable {
    private var data: Data?

    init(data: Data? = nil) {
        self.data = data
    }

    func load() -> Data? { data }
    func save(_ data: Data) { self.data = data }
    func clear() { data = nil }
}

/// ファイルに保存する。
///
/// UserDefaults ではなくファイルにするのは、**記録が消えては困る**ため。
/// UserDefaults は容量の想定が小さく、OS が整理する対象にもなる。
struct FilePendingStore: PendingStore {
    let url: URL

    init(filename: String = "pending.json") {
        let dir = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        url = dir.appendingPathComponent(filename)
    }

    func load() -> Data? { try? Data(contentsOf: url) }

    func save(_ data: Data) {
        // .atomic: 書き込み中に落ちても、途中まで書かれたファイルが残らない
        try? data.write(to: url, options: .atomic)
    }

    func clear() { try? FileManager.default.removeItem(at: url) }
}

/// 送信待ちのキュー。
final class PendingQueue: @unchecked Sendable {
    private let store: PendingStore
    private let lock = NSLock()

    init(store: PendingStore) {
        self.store = store
    }

    /// 積む。同じ id は上書きする（同じ端末からの訂正とみなす）。
    func push(_ item: PendingSet) {
        lock.lock()
        defer { lock.unlock() }

        var items = readUnlocked()
        if let i = items.firstIndex(where: { $0.id == item.id }) {
            items[i] = item
        } else {
            items.append(item)
        }
        writeUnlocked(items)
    }

    /// 積んである全件を、積んだ順に返す。
    func all() -> [PendingSet] {
        lock.lock()
        defer { lock.unlock() }

        return readUnlocked()
    }

    /// 送れたものを取り除く。
    func remove(ids: [UUID]) {
        lock.lock()
        defer { lock.unlock() }

        let drop = Set(ids)
        writeUnlocked(readUnlocked().filter { !drop.contains($0.id) })
    }

    var count: Int { all().count }

    func clear() {
        lock.lock()
        defer { lock.unlock() }

        store.clear()
    }

    /// **壊れていたら捨てて空から始める。** 形式が変わったときや書き込みが
    /// 途中で切れたときに、アプリが起動できなくなる方が困る。
    private func readUnlocked() -> [PendingSet] {
        guard let data = store.load() else { return [] }

        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601

        return (try? decoder.decode([PendingSet].self, from: data)) ?? []
    }

    private func writeUnlocked(_ items: [PendingSet]) {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601

        guard let data = try? encoder.encode(items) else { return }
        store.save(data)
    }
}
