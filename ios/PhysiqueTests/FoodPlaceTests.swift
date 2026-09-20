import Foundation
import Testing

@testable import PhysiqueCore

/// 適当な緯度経度。**実在の場所を書かない**（公開リポジトリ）
private let a = Coordinate(lat: 35.000, lng: 139.000)
/// a から約 90m。同じ店とみなしたい距離
private let aNear = Coordinate(lat: 35.0008, lng: 139.000)
/// a から約 1.1km。別の場所
private let far = Coordinate(lat: 35.010, lng: 139.000)

private let ramen = UUID(uuidString: "11111111-1111-1111-1111-111111111111")!
private let protein = UUID(uuidString: "22222222-2222-2222-2222-222222222222")!
private let egg = UUID(uuidString: "33333333-3333-3333-3333-333333333333")!

// MealModelTests の同名ヘルパは fileprivate なのでここにも置く
private func api(_ responses: [(String, Int)]) -> (APIClient, FakeTransport) {
    let t = FakeTransport()
    t.responses = responses.map { (Data($0.0.utf8), $0.1) }

    return (APIClient(baseURL: URL(string: "http://api.test")!, transport: t), t)
}

private let emptyMeals = (#"{"items":[]}"#, 200)
private let targets = ("""
{"date":"2026-09-21","target":{"kcal":2400,"proteinG":180,"fatG":70,"carbG":250},
 "consumed":{"kcal":0,"proteinG":0,"fatG":0,"carbG":0}}
""", 200)

@Suite("座標")
struct CoordinateTests {
    @Test("距離を出せる")
    func distance() {
        // 緯度 0.001 度 ≒ 111m
        #expect(abs(a.meters(to: Coordinate(lat: 35.001, lng: 139.000)) - 111) < 3)
        #expect(a.meters(to: a) == 0)
    }

    @Test("**小数第3位に丸める。** 生の座標を端末に貯めない")
    func rounds() {
        let c = Coordinate(lat: 35.123456, lng: 139.987654).coarse()

        #expect(c.lat == 35.123)
        #expect(c.lng == 139.988)
    }
}

@Suite("よく行く場所")
struct FoodPlacesTests {
    @Test("使った場所を覚える")
    func records() {
        var p = FoodPlaces()

        p.record(ramen, at: a)

        #expect(p.score(ramen, near: a) == 1)
    }

    @Test("同じ場所で使うと回数が増える")
    func counts() {
        var p = FoodPlaces()
        p.record(ramen, at: a)
        p.record(ramen, at: aNear)

        // 丸めたあとが同じなので1件にまとまる
        #expect(p.score(ramen, near: a) == 2)
    }

    @Test("**離れた場所では 0**")
    func farAway() {
        var p = FoodPlaces()
        p.record(ramen, at: a)

        #expect(p.score(ramen, near: far) == 0)
    }

    @Test("覚えていない項目は 0")
    func unknown() {
        #expect(FoodPlaces().score(ramen, near: a) == 0)
    }

    @Test("近くで使ったものが先頭に出る")
    func ordersNearFirst() {
        var p = FoodPlaces()
        p.record(ramen, at: a)

        // サーバの順は egg → protein → ramen（よく使う順）
        let ordered = p.order([egg, protein, ramen], id: { $0 }, near: a)

        #expect(ordered == [ramen, egg, protein])
    }

    @Test("近くのものが複数あれば回数順")
    func ordersByCount() {
        var p = FoodPlaces()
        p.record(egg, at: a)
        p.record(ramen, at: a)
        p.record(ramen, at: a)

        let ordered = p.order([egg, protein, ramen], id: { $0 }, near: a)

        #expect(ordered == [ramen, egg, protein])
    }

    @Test("**残りはサーバの順のまま。** 並べ替えで崩さない")
    func keepsServerOrder() {
        var p = FoodPlaces()
        p.record(ramen, at: a)

        let ordered = p.order([egg, protein, ramen], id: { $0 }, near: a)

        #expect(Array(ordered.dropFirst()) == [egg, protein])
    }

    @Test("場所が分からなければ並びを変えない")
    func noLocation() {
        var p = FoodPlaces()
        p.record(ramen, at: a)

        #expect(p.order([egg, protein, ramen], id: { $0 }, near: nil) == [egg, protein, ramen])
    }

    @Test("離れた場所では並びを変えない")
    func farKeepsOrder() {
        var p = FoodPlaces()
        p.record(ramen, at: a)

        #expect(p.order([egg, protein, ramen], id: { $0 }, near: far) == [egg, protein, ramen])
    }

    @Test("保存して読み戻せる")
    func roundTrip() throws {
        var p = FoodPlaces()
        p.record(ramen, at: a)
        p.record(ramen, at: a)

        let back = try JSONDecoder().decode(FoodPlaces.self, from: JSONEncoder().encode(p))

        #expect(back.score(ramen, near: a) == 2)
    }

    @Test("**丸めた座標しか持たない**")
    func storesOnlyCoarse() throws {
        var p = FoodPlaces()
        p.record(ramen, at: Coordinate(lat: 35.123456, lng: 139.987654))

        let json = String(decoding: try JSONEncoder().encode(p), as: UTF8.self)

        #expect(!json.contains("35.123456"))
        #expect(json.contains("35.123"))
    }
}

/// 位置情報の偽物。**実機でしか本物は動かない**ので、判断はここで見る。
final class FakeLocation: LocationSource, @unchecked Sendable {
    var permission: LocationPermission
    var next: Coordinate?
    private(set) var requestCount = 0

    init(permission: LocationPermission = .notDetermined, at: Coordinate? = nil) {
        self.permission = permission
        next = at
    }

    func request() async -> LocationPermission {
        requestCount += 1
        if permission == .notDetermined { permission = .granted }

        return permission
    }

    func current() async -> Coordinate? {
        permission == .granted ? next : nil
    }
}

@Suite("近い順に並べる")
@MainActor
struct NearbyOrderTests {
    private let items = #"""
    {"items":[
      {"id":"33333333-3333-3333-3333-333333333333","name":"ゆで卵","components":[],
       "usedCount":9,"createdAt":"2026-09-21T00:00:00Z","updatedAt":"2026-09-21T00:00:00Z"},
      {"id":"11111111-1111-1111-1111-111111111111","name":"ラーメン","components":[],
       "usedCount":1,"createdAt":"2026-09-21T00:00:00Z","updatedAt":"2026-09-21T00:00:00Z"}
    ]}
    """#

    private func model(_ loc: FakeLocation) async -> (MealModel, FakeTransport) {
        let (client, t) = api([emptyMeals, targets, (items, 200)])
        let m = MealModel(api: client, date: "2026-09-21", location: loc)
        await m.load()
        await m.loadFoodItems()

        return (m, t)
    }

    @Test("**開いただけでは権限を聞かない**")
    func doesNotAsk() async {
        let loc = FakeLocation()
        let (m, _) = await model(loc)

        #expect(loc.requestCount == 0)
        #expect(!m.nearbyOn)
        // サーバの順（よく使う順）のまま
        #expect(m.foodItems.map(\.name) == ["ゆで卵", "ラーメン"])
    }

    @Test("有効にすると権限を聞く")
    func asks() async {
        let loc = FakeLocation(at: a)
        let (m, _) = await model(loc)

        await m.enableNearby()

        #expect(loc.requestCount == 1)
        #expect(m.nearbyOn)
    }

    @Test("**断られても今までどおり使える**")
    func denied() async {
        let loc = FakeLocation(permission: .denied, at: a)
        let (m, _) = await model(loc)

        await m.enableNearby()

        #expect(!m.nearbyOn)
        #expect(m.foodItems.map(\.name) == ["ゆで卵", "ラーメン"])
    }

    @Test("その場で使ったものが次から上に出る")
    func ordersNearFirst() async {
        let loc = FakeLocation(at: a)
        let (m, _) = await model(loc)
        await m.enableNearby()

        // ラーメンをこの場所で選ぶ
        m.pickFood(m.foodItems.first { $0.name == "ラーメン" }!)
        await m.loadFoodItems()

        #expect(m.foodItems.map(\.name) == ["ラーメン", "ゆで卵"])
    }

    @Test("**離れた場所では並びが戻る**")
    func farAway() async {
        let loc = FakeLocation(at: a)
        let (m, _) = await model(loc)
        await m.enableNearby()
        m.pickFood(m.foodItems.first { $0.name == "ラーメン" }!)

        loc.next = far
        await m.enableNearby()
        await m.loadFoodItems()

        #expect(m.foodItems.map(\.name) == ["ゆで卵", "ラーメン"])
    }

    @Test("**緯度経度を API に送らない**")
    func neverSendsCoordinates() async {
        let loc = FakeLocation(at: a)
        let (m, t) = await model(loc)
        await m.enableNearby()
        m.pickFood(m.foodItems[0])

        for req in t.requests {
            let url = req.url?.absoluteString ?? ""
            #expect(!url.contains("35.0"))
            #expect(!url.contains("139.0"))

            let body = req.httpBody.map { String(decoding: $0, as: UTF8.self) } ?? ""
            #expect(!body.contains("lat"))
            #expect(!body.contains("lng"))
        }
    }
}
