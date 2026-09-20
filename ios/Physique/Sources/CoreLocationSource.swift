#if canImport(CoreLocation)
import CoreLocation
import Foundation

/// CoreLocation から実際に読む実装（要件 N-08）。
///
/// **判断はここに書かない。** 並べ替えは `FoodPlaces`、聞くかどうかは
/// `MealModel` が持っていて、そちらはテストがある（`HealthKitSource` と同じ扱い）。
///
/// **1回ぶんの場所しか要らない。** 追跡もジオフェンスもしないので
/// `requestLocation()` で足りる。常時監視すると電池を食うし、
/// 「いつも許可」を要求する理由も無い。
final class CoreLocationSource: NSObject, LocationSource, CLLocationManagerDelegate,
                                @unchecked Sendable {
    private let manager = CLLocationManager()
    /// requestLocation は delegate で返るので、continuation を持って待つ
    private var waiting: CheckedContinuation<Coordinate?, Never>?
    private var asking: CheckedContinuation<LocationPermission, Never>?
    private let lock = NSLock()

    override init() {
        super.init()
        manager.delegate = self
        // **店を区別できればよい。** 精度を上げるほど時間も電池も食う
        manager.desiredAccuracy = kCLLocationAccuracyHundredMeters
    }

    var permission: LocationPermission {
        switch manager.authorizationStatus {
        case .notDetermined: .notDetermined
        case .authorizedWhenInUse, .authorizedAlways: .granted
        default: .denied
        }
    }

    func request() async -> LocationPermission {
        guard permission == .notDetermined else { return permission }

        return await withCheckedContinuation { c in
            lock.withLock { asking = c }
            manager.requestWhenInUseAuthorization()
        }
    }

    func current() async -> Coordinate? {
        guard permission == .granted else { return nil }

        return await withCheckedContinuation { c in
            lock.withLock { waiting = c }
            manager.requestLocation()
        }
    }

    // MARK: - CLLocationManagerDelegate

    func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        // notDetermined のまま呼ばれることがある（初回の delegate 設定時）。
        // ダイアログの答えではないので、待っているときだけ返す
        guard permission != .notDetermined else { return }
        resumeAsking(with: permission)
    }

    func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        let c = locations.last.map {
            Coordinate(lat: $0.coordinate.latitude, lng: $0.coordinate.longitude)
        }
        resumeWaiting(with: c)
    }

    func locationManager(_ manager: CLLocationManager, didFailWithError error: Error) {
        // **握る。** 場所が取れないのは「並び順が変わらない」だけで、
        // 記録そのものは続けられる
        resumeWaiting(with: nil)
    }

    private func resumeWaiting(with c: Coordinate?) {
        let cont = lock.withLock { () -> CheckedContinuation<Coordinate?, Never>? in
            defer { waiting = nil }

            return waiting
        }
        cont?.resume(returning: c)
    }

    private func resumeAsking(with p: LocationPermission) {
        let cont = lock.withLock { () -> CheckedContinuation<LocationPermission, Never>? in
            defer { asking = nil }

            return asking
        }
        cont?.resume(returning: p)
    }
}
#endif
