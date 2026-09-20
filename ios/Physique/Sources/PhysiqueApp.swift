import SwiftUI

/// ジムでの入力に特化したクライアント（ADR-0009）。
///
/// **Web と役割を分ける。** Web は入力も分析もできる汎用クライアントで、
/// iOS の存在理由は「ジムでの入力速度」と「HealthKit 連携」。
@main
struct PhysiqueApp: App {
    @State private var auth: AuthModel

    init() {
        #if DEBUG
        // **UI テストのときだけ差し替える。** 本番の経路は変えず、
        // 既存の protocol に偽物を挿す（UITestSupport）
        if UITestSupport.isActive {
            _auth = State(initialValue: UITestSupport.makeAuth())

            return
        }
        #endif

        _auth = State(initialValue: AuthModel(
            auth: AuthClient(
                // 未設定なら明らかに失敗する URL にする。黙って localhost を
                // 叩いて「なぜか繋がらない」にするより、設定漏れが分かる方がいい
                url: AppConfig.supabaseURL ?? URL(string: "https://supabase-url-未設定.invalid")!,
                anonKey: AppConfig.supabaseAnonKey ?? ""
            ),
            store: KeychainSessionStore()
        ))
    }

    var body: some Scene {
        WindowGroup {
            if auth.isSignedIn {
                MainTabs(auth: auth)
            } else {
                LoginView(auth: auth)
            }
        }
    }
}

private struct MainTabs: View {
    let auth: AuthModel

    /// API はログイン中のトークンを都度取りに行く（APIClient.TokenProvider）
    private var api: APIClient {
        #if DEBUG
        if UITestSupport.isActive { return UITestSupport.makeAPI() }
        #endif

        return APIClient(
            baseURL: AppConfig.apiBaseURL,
            tokenProvider: { [auth] in try await auth.accessToken() }
        )
    }

    var body: some View {
        // **食事を先頭にする。** 開く頻度が違う —— 食事は1日4〜6回、
        // トレーニングは週3〜5回。起動直後に出る画面が食事である方が、
        // 合計の操作回数が減る。体組成は朝に触るものなのでタブを分ける
        TabView {
            MealView(api: api)
                .tabItem { Label("食事", systemImage: "fork.knife") }
            LogView(api: api)
                .tabItem { Label("記録", systemImage: "dumbbell") }
            BodyView(api: api, health: HealthKitSource())
                .tabItem { Label("体組成", systemImage: "figure") }
            SettingsView(auth: auth)
                .tabItem { Label("設定", systemImage: "gearshape") }
        }
    }
}

/// ログアウトだけを置く。設定そのものは Web で触る（ADR-0009）
private struct SettingsView: View {
    let auth: AuthModel

    var body: some View {
        NavigationStack {
            List {
                Section {
                    Button("ログアウト", role: .destructive) {
                        Task { await auth.signOut() }
                    }
                } footer: {
                    Text("フェーズや栄養の設定は Web で行う")
                }
            }
            .navigationTitle("設定")
        }
    }
}
