import SwiftUI

/// ジムでの入力に特化したクライアント（ADR-0009）。
///
/// **Web と役割を分ける。** Web は入力も分析もできる汎用クライアントで、
/// iOS の存在理由は「ジムでの入力速度」と「HealthKit 連携」。
@main
struct PhysiqueApp: App {
    var body: some Scene {
        WindowGroup {
            LogView()
        }
    }
}
