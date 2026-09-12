import Foundation
import Testing

@testable import PhysiqueCore

private func pending(setNo: Int = 1, id: UUID = UUID(), weightKg: Double = 80) -> PendingSet {
    PendingSet(
        id: id, date: "2026-09-13", exerciseId: UUID(),
        setNo: setNo, weightKg: weightKg, reps: 5, rir: 2, queuedAt: Date()
    )
}

@Suite("PendingQueue")
struct PendingQueueTests {
    @Test("積んだものを順に返す")
    func order() {
        let q = PendingQueue(store: MemoryPendingStore())
        q.push(pending(setNo: 1))
        q.push(pending(setNo: 2))

        #expect(q.all().map(\.setNo) == [1, 2])
    }

    /// 二重に積まれると、復帰時に同じセットが2本入る
    @Test("同じ id は二重に積まない")
    func idempotent() {
        let q = PendingQueue(store: MemoryPendingStore())
        let id = UUID()
        q.push(pending(id: id, weightKg: 80))
        q.push(pending(id: id, weightKg: 999))

        let all = q.all()
        #expect(all.count == 1)
        // 後から来た内容で上書きする（同じ端末の訂正とみなす）
        #expect(all[0].weightKg == 999)
    }

    @Test("送れたものを取り除く")
    func removeSent() {
        let q = PendingQueue(store: MemoryPendingStore())
        let a = pending(setNo: 1)
        let b = pending(setNo: 2)
        q.push(a)
        q.push(b)

        q.remove(ids: [a.id])

        #expect(q.all().map(\.id) == [b.id])
    }

    @Test("存在しない id の削除は何もしない")
    func removeUnknown() {
        let q = PendingQueue(store: MemoryPendingStore())
        q.push(pending())
        q.remove(ids: [UUID()])

        #expect(q.count == 1)
    }

    @Test("空にできる")
    func clear() {
        let q = PendingQueue(store: MemoryPendingStore())
        q.push(pending())
        q.clear()

        #expect(q.count == 0)
    }

    /// 保存の形式が壊れていても、起動できなくなってはいけない
    @Test("壊れた保存内容は捨てて空から始める")
    func brokenData() {
        let q = PendingQueue(store: MemoryPendingStore(data: Data("{壊れている".utf8)))

        #expect(q.all().isEmpty)
    }

    @Test("配列でない保存内容も捨てる")
    func notAnArray() {
        let q = PendingQueue(store: MemoryPendingStore(data: Data(#"{"not":"an array"}"#.utf8)))

        #expect(q.all().isEmpty)
    }

    @Test("保存して読み直せる")
    func roundTrip() {
        let store = MemoryPendingStore()
        let a = pending(setNo: 3, weightKg: 102.5)
        PendingQueue(store: store).push(a)

        let reopened = PendingQueue(store: store)
        let all = reopened.all()
        #expect(all.count == 1)
        #expect(all[0].setNo == 3)
        #expect(all[0].weightKg == 102.5)
    }
}
