import Foundation

/// 位置情報の許可の状態（要件 N-08）。
///
/// CoreLocation の `CLAuthorizationStatus` を3つに畳んである。
/// **アプリが分岐したいのはこの3つだけ**で、`restricted` と `denied` は
/// 「使えない」として同じ扱いでよい。
enum LocationPermission: Sendable, Equatable {
    /// まだ聞いていない。**勝手に聞かない**
    case notDetermined
    case denied
    case granted
}

/// いまいる場所を返すもの。
///
/// **実装を差し替えられるようにする。** CoreLocation はシミュレータでも
/// 実機でも聞き方が変わるので、判断のテストは偽物で回す
/// （`HealthSource` と同じ形）。
protocol LocationSource: Sendable {
    var permission: LocationPermission { get }

    /// 許可を聞く。**呼んだときだけ**ダイアログが出る
    func request() async -> LocationPermission

    /// いまいる場所。許可が無ければ nil
    func current() async -> Coordinate?
}

/// 位置情報を使わない実装。**既定。**
///
/// 権限を持たない画面や、位置を使う理由が無いときに挿す。
struct NoLocation: LocationSource {
    var permission: LocationPermission { .denied }
    func request() async -> LocationPermission { .denied }
    func current() async -> Coordinate? { nil }
}
