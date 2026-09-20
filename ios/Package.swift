// swift-tools-version: 6.2
import PackageDescription

/// ロジックを `swift test` で回せるようにするためのパッケージ。
///
/// **アプリ本体（.xcodeproj）と同じソースを指している。** Xcode を開かずに
/// テストを回せる方が、CI でも手元でも速い。UI は SwiftUI のプレビューと
/// シミュレータで確認する。
let package = Package(
    name: "PhysiqueCore",
    platforms: [.iOS(.v17), .macOS(.v14)],
    products: [
        .library(name: "PhysiqueCore", targets: ["PhysiqueCore"]),
    ],
    targets: [
        .target(
            name: "PhysiqueCore",
            path: "Physique/Sources",
            // UI は SwiftUI で、テスト対象はロジックだけ
            exclude: [
                "Info.plist", "PhysiqueApp.swift",
                // UI と HealthKit は iOS 専用。macOS ビルドで落ちる
                "LogView.swift", "BodyView.swift", "HealthKitSource.swift", "LoginView.swift", "MealView.swift",
                // UIApplication は iOS 専用
                "KeyboardDismiss.swift",
                // UI テスト専用。swift test では使わない
                "UITestSupport.swift",
            ]
        ),
        .testTarget(
            name: "PhysiqueCoreTests",
            dependencies: ["PhysiqueCore"],
            path: "PhysiqueTests"
        ),
    ]
)
