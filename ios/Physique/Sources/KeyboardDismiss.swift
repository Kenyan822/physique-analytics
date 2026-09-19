import SwiftUI

extension View {
    /// 入力欄の外をタップしたらキーボードを閉じる。
    ///
    /// **3経路で閉じられるようにする。** 数値を打ったあとキーボードが残ると、
    /// 下の項目と記録ボタンが隠れる。食事は1日4〜6回入れるので、
    /// ここで1タップ増えるのが効く。
    ///
    /// | | いつ効くか |
    /// |---|---|
    /// | タップ | 入力欄でない場所を押したとき |
    /// | スクロール | 指を下ろした時点で追従して閉じる |
    /// | 「完了」 | 上2つが効かない場所でも確実に閉じられる逃げ道 |
    ///
    /// **`@FocusState` を経由しない。** 画面ごとに `@FocusState` の型が違うので、
    /// 共通化するには「いま誰が first responder か」を知らずに降ろせる必要がある。
    func dismissesKeyboardOnTap() -> some View {
        modifier(DismissKeyboardOnTap())
    }
}

private struct DismissKeyboardOnTap: ViewModifier {
    func body(content: Content) -> some View {
        content
            // .interactively にすると、指を下ろした量に追従して閉じる。
            // .immediately は少し触れただけで消えて、行を選びたいだけのときに邪魔
            .scrollDismissesKeyboard(.interactively)
            // **Form の中の TextField や Button は自分でタップを消費する。**
            // ここに来るのは「どのコントロールでもない場所」を押したときだけ
            .onTapGesture(perform: dismissKeyboard)
            .toolbar {
                ToolbarItemGroup(placement: .keyboard) {
                    Spacer()
                    Button("完了", action: dismissKeyboard)
                }
            }
    }
}

/// いま編集中の欄が何かを知らずにキーボードを降ろす。
///
/// `resignFirstResponder` を宛先なしで送ると、responder chain を辿って
/// 編集中のものに届く。
private func dismissKeyboard() {
    UIApplication.shared.sendAction(
        #selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil
    )
}
