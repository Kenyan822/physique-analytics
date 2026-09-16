import SwiftUI

/// ログイン画面。
///
/// **ジムで開くことは無い**ので、入力速度より確実さを優先する。
struct LoginView: View {
    @Bindable var auth: AuthModel

    @State private var email = ""
    @State private var password = ""
    @FocusState private var focus: Field?

    private enum Field { case email, password }

    private var canSubmit: Bool {
        !email.trimmingCharacters(in: .whitespaces).isEmpty && !password.isEmpty && !auth.isWorking
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Spacer()

            VStack(alignment: .leading, spacing: 4) {
                Text("physique").font(.title.bold())
                Text("記録を見るにはログインが要る").font(.subheadline).foregroundStyle(.secondary)
            }

            VStack(spacing: 12) {
                TextField("メールアドレス", text: $email)
                    .textContentType(.username)
                    .keyboardType(.emailAddress)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .focused($focus, equals: .email)
                    .submitLabel(.next)
                    .onSubmit { focus = .password }

                SecureField("パスワード", text: $password)
                    .textContentType(.password)
                    .focused($focus, equals: .password)
                    .submitLabel(.go)
                    .onSubmit { submit() }
            }
            .textFieldStyle(.roundedBorder)

            Button(action: submit) {
                if auth.isWorking {
                    ProgressView().frame(maxWidth: .infinity)
                } else {
                    Text("ログイン").bold().frame(maxWidth: .infinity)
                }
            }
            .buttonStyle(.borderedProminent)
            .controlSize(.large)
            .disabled(!canSubmit)

            if let message = auth.errorMessage {
                Text(message)
                    .font(.footnote)
                    .foregroundStyle(.red)
                    .accessibilityAddTraits(.isStaticText)
            }

            Spacer()
        }
        .padding(24)
        .onAppear { focus = .email }
    }

    private func submit() {
        guard canSubmit else { return }
        Task { await auth.signIn(email: email.trimmingCharacters(in: .whitespaces), password: password) }
    }
}
