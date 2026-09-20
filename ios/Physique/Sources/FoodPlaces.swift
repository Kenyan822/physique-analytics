import Foundation

/// 緯度経度（要件 N-08）。
///
/// **サーバには送らない。** 端末内で並べ替えに使うだけ
/// （[docs/01-要件定義.md §4-C](../../../docs/01-要件定義.md)）。
struct Coordinate: Hashable, Sendable {
    let lat: Double
    let lng: Double

    /// 保存用に丸める。**生の座標を端末に貯めない。**
    ///
    /// 小数第3位は緯度で約 111m、東経 139 度あたりの経度で約 91m。
    /// 店を区別するには足りるし、消し忘れたときの被害も小さい。
    func coarse() -> Coordinate {
        Coordinate(lat: round(lat), lng: round(lng))
    }

    /// 2点間の距離（m）。haversine。
    ///
    /// **平面近似にしない。** 数式が短いのと、緯度によって誤差が変わる
    /// のを気にせずに済むのとで、こちらの方が考えることが少ない。
    func meters(to other: Coordinate) -> Double {
        let earth = 6_371_000.0
        let dLat = radians(other.lat - lat)
        let dLng = radians(other.lng - lng)

        let a = sin(dLat / 2) * sin(dLat / 2)
            + cos(radians(lat)) * cos(radians(other.lat)) * sin(dLng / 2) * sin(dLng / 2)

        return 2 * earth * atan2(sqrt(a), sqrt(1 - a))
    }

    private func round(_ v: Double) -> Double { (v * 1000).rounded() / 1000 }
    private func radians(_ deg: Double) -> Double { deg * .pi / 180 }
}

/// 「どこで何を食べたか」の端末内の記録（要件 N-08）。
///
/// **サーバには持たせない。** `openapi.yaml` にも DB にも出てこない。
struct FoodPlaces: Codable, Sendable, Equatable {
    /// 同じ場所とみなす距離。店1軒ぶんの見当
    static let nearMeters = 200.0

    /// 1件 = 「この項目を、この丸めた座標で、何回使ったか」
    struct Use: Codable, Hashable, Sendable {
        let itemID: UUID
        let lat: Double
        let lng: Double
        var count: Int
    }

    private(set) var uses: [Use] = []

    /// 使ったことを覚える。**丸めてから持つ。**
    mutating func record(_ itemID: UUID, at where_: Coordinate) {
        let c = where_.coarse()

        if let i = uses.firstIndex(where: {
            $0.itemID == itemID && $0.lat == c.lat && $0.lng == c.lng
        }) {
            uses[i].count += 1

            return
        }

        uses.append(Use(itemID: itemID, lat: c.lat, lng: c.lng, count: 1))
    }

    /// いまいる場所の近くで、この項目を使った回数。
    func score(_ itemID: UUID, near here: Coordinate) -> Int {
        uses
            .filter {
                $0.itemID == itemID
                    && Coordinate(lat: $0.lat, lng: $0.lng).meters(to: here) <= Self.nearMeters
            }
            .reduce(0) { $0 + $1.count }
    }

    /// 近くで使ったものを先頭に出す。
    ///
    /// **残りはもとの順のまま。** サーバが「よく使う順」で返しているので、
    /// 近くに心当たりが無い項目まで並べ替える理由が無い。
    ///
    /// `sorted(by:)` は安定ではないので、自分で前後に分ける。
    func order<T>(_ items: [T], id: (T) -> UUID, near here: Coordinate?) -> [T] {
        guard let here else { return items }

        var near: [(item: T, score: Int, at: Int)] = []
        var rest: [T] = []

        for (i, item) in items.enumerated() {
            let s = score(id(item), near: here)
            if s > 0 {
                near.append((item, s, i))
            } else {
                rest.append(item)
            }
        }

        // 回数が同じなら、もとの順を保つ
        near.sort { $0.score == $1.score ? $0.at < $1.at : $0.score > $1.score }

        return near.map(\.item) + rest
    }
}

/// 端末内の保存先（要件 N-08）。
///
/// **サーバに持たせない**ので、消えたら並び順が戻るだけ。記録そのものは
/// サーバにあるため、`PendingQueue` のような堅さは要らない。
struct FoodPlaceStore: Sendable {
    let url: URL?

    /// 既定は Application Support の `food-places.json`。
    /// テストは `url: nil`（保存しない）か一時ファイルを渡す
    init(filename: String? = "food-places.json") {
        guard let filename else {
            url = nil

            return
        }

        let dir = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        url = dir.appendingPathComponent(filename)
    }

    init(url: URL?) {
        self.url = url
    }

    func load() -> FoodPlaces {
        guard let url, let data = try? Data(contentsOf: url) else { return FoodPlaces() }

        return (try? JSONDecoder().decode(FoodPlaces.self, from: data)) ?? FoodPlaces()
    }

    func save(_ places: FoodPlaces) {
        guard let url, let data = try? JSONEncoder().encode(places) else { return }
        try? data.write(to: url, options: .atomic)
    }
}
