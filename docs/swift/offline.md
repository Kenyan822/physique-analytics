# オフラインでも記録が消えない仕組み（要件 T-07）

**ジムは電波が悪い。** 地下や鉄骨の建物で圏外になる。
セットを記録するたびに通信の成否を待たせると使い物にならない。

## 考え方

**記録は「積んだ時点」で確定させる。** 送信はその後の非同期な作業で、
成否は表示で伝えるだけ。

```swift
// **積んだ時点で記録は確定。** 送信の成否は表示で伝えるだけにする。
// 「失敗したら入力し直し」では電波の悪いジムで使えない
queue.push(item)
logged.append(item)
startRest(for: exerciseId)

await flush()
updatePendingMessage()
```

`queue.push` はファイル書き込みで、ミリ秒で終わる。
`logged.append` で画面にも即座に出る。`await flush()` が失敗しても、
**ユーザーから見れば記録は終わっている**。

## 保存先はファイル

```swift
/// UserDefaults ではなくファイルにするのは、**記録が消えては困る**ため。
/// UserDefaults は容量の想定が小さく、OS が整理する対象にもなる。
struct FilePendingStore: PendingStore {
    let url: URL

    init(filename: String = "pending.json") {
        let dir = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        url = dir.appendingPathComponent(filename)
    }
```

| 置き場所 | 問題 |
|---|---|
| `UserDefaults` | 容量の想定が小さい。OS が整理する |
| `.cachesDirectory` | **OS が空き容量のために消す** |
| `.documentDirectory` | Files アプリに見える。ユーザーが消せる |
| `.applicationSupportDirectory` | ✅ アプリのデータ置き場。消されない |

### `.atomic` で書く

```swift
func save(_ data: Data) {
    // .atomic: 書き込み中に落ちても、途中まで書かれたファイルが残らない
    try? data.write(to: url, options: .atomic)
}
```

一時ファイルに書いてから `rename` する。`rename` はファイルシステムの
アトミック操作なので、**途中の状態が観測されない**。

これが無いと、書き込み中にアプリが落ちたとき JSON が壊れて
次回起動時に全件失う。

## 壊れていたら捨てる

```swift
/// **壊れていたら捨てて空から始める。** 形式が変わったときや書き込みが
/// 途中で切れたときに、アプリが起動できなくなる方が困る。
private func readUnlocked() -> [PendingSet] {
    guard let data = store.load() else { return [] }

    let decoder = JSONDecoder()
    decoder.dateDecodingStrategy = .iso8601

    return (try? decoder.decode([PendingSet].self, from: data)) ?? []
}
```

**`try?` で握りつぶすのは意図的。** ここで throw すると、
壊れたファイルが1つあるだけでアプリが記録画面を開けなくなる。

失うのは未送信分だけで、送信済みの記録はサーバにある。
「起動できない」より「未送信が消える」方がまし、という判断。

## `NSLock` で守る

```swift
final class PendingQueue: @unchecked Sendable {
    private let store: PendingStore
    private let lock = NSLock()

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
```

**read-modify-write なので、ロックが要る。** 2つのタスクが同時に `push` すると
片方の追記が消える。

`actor` にしなかった理由は [concurrency.md](concurrency.md#unchecked-sendable-は自分で守る宣言)。
同期メソッドのまま使いたかった。

`readUnlocked` / `writeUnlocked` という命名で「**ロックを取らない**」ことを
明示している。うっかり `push` の中から `all()` を呼ぶと
`NSLock` は再入不可なのでデッドロックする。

## 再送の冪等性

```swift
struct PendingSet: Codable, Identifiable, Equatable, Sendable {
    /// クライアントが生成する UUID。再送の冪等性に使う（ADR-0014）
    let id: UUID
```

**id をクライアントで振る。** サーバが採番すると、
「送ったが応答が失われた」ときに再送すると2件になる。

```swift
_ = try await api.createSet(
    sessionId: session.id,
    WorkoutSetInput(
        id: item.id, ...      // ← 同じ id を送る
    )
)
```

サーバ側は同じ id を受けたら弾く（[ADR-0014](../adr/0014-sync-conflict-resolution.md)）。

## `flush` の3分岐

```swift
func flush() async {
    var sent: [UUID] = []

    for item in queue.all() {
        do {
            let session = try await ensureSession()
            _ = try await api.createSet(sessionId: session.id, ...)
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
```

| 結果 | 扱い |
|---|---|
| 成功 | `sent` に入れて後で消す |
| **409 / 422「既にある」** | **成功扱い。** 前回届いていた |
| それ以外の失敗 | 残す。**`break` して以降を試さない** |

### なぜ `break` するか

`continue` だと、圏外のとき**全件に対して通信を試して全部失敗する**。
1件失敗した時点で通信できないと判断して抜ける。

副次的に**順序も保たれる**。セット番号は連番なので、
途中だけ送れて歯抜けになるのを避けたい。

### `catch let e as APIError where e.isAlreadyRecorded`

`where` 付きの catch。**型が合い、かつ条件を満たすときだけ**この節に入る。
条件を満たさない `APIError` は下の `catch` に落ちる。

Go だと `errors.As` してから `if` を書くところが、1行で書ける。

## セット番号は最大値 + 1

```swift
/// 次のセット番号。**件数ではなく最大値 + 1**。
/// 途中のセットを消したあとに件数で決めると、既存の番号と衝突する
var nextSetNo: Int {
    guard let id = selectedExerciseId else { return 1 }

    return (logged.filter { $0.exerciseId == id }.map(\.setNo).max() ?? 0) + 1
}
```

3セット記録して2セット目を消すと件数は 2。次を 3 にすると既存と衝突する。

### 起動時にサーバから読み直す

```swift
// その日の記録を先に読む。画面を開き直したときにセット番号が
// 1 に戻ると、既に記録した番号と衝突して入力できない
let sessions = try await api.listSessions(from: date, to: date, limit: 1)
logged = (sessions.first?.sets ?? []).map { PendingSet(...) }
```

`logged` はローカルの表示用配列なので、アプリを再起動すると空になる。
サーバの記録を `PendingSet` に詰め替えて埋めている。

**`queue` とは別物。** `logged` は「今日記録した全部」、
`queue` は「まだ送れていない分」。

## 読み込みに失敗しても入力できる

```swift
} catch {
    // 読めなくても入力はできる。積んでおけば復帰時に送られる
    report(error)
}
```

`load()` が失敗しても画面は使える。種目一覧が空になるので選べないが、
**すでに選んである状態から続きを記録することはできる**。

## 未送信の件数を出す

```swift
private func updatePendingMessage() {
    let n = queue.count
    pendingMessage = n > 0 ? "未送信 \(n) 件。オンライン復帰時に送る" : ""
}
```

黙って溜めない。**何件残っているかを見せる**。

## まだ無いもの

- **自動再送のきっかけ**。いまは `load()` と `record()` のときだけ `flush` する。
  `NWPathMonitor` で復帰を検知して送るのが本来
- **バックグラウンド送信**。アプリを閉じている間は送られない
- **古い記録の扱い**。何日も送れないまま溜まったときの上限が無い
