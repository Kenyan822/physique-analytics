import SwiftUI

/// ジムでの入力に特化したクライアント（ADR-0009）。
///
/// **Web と役割を分ける。** Web は入力も分析もできる汎用クライアントで、
/// iOS の存在理由は「ジムでの入力速度」と「HealthKit 連携」。
@main
struct PhysiqueApp: App {
    var body: some Scene {
        WindowGroup {
            // ジムで開くのは記録画面。体組成は朝に触るものなのでタブを分ける
            TabView {
                LogView()
                    .tabItem { Label("記録", systemImage: "dumbbell") }
                BodyView(api: APIClient(baseURL: AppConfig.apiBaseURL), health: HealthKitSource())
                    .tabItem { Label("体組成", systemImage: "figure") }
            }
        }
    }
}
