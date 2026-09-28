import Combine
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
    /// | スクロール | 指を下ろした時点で追従して閉じる |
    /// | タップ | **読むだけの行**を押したとき（`dismissesKeyboardWhenTapped`） |
    /// | 「完了」／「次へ」 | 上2つが効かない場所でも確実に閉じられる逃げ道 |
    ///
    /// **画面全体にタップ判定を付けない。** コントロールからタップを奪う（#229）。
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
    }
}

extension View {
    /// **押しても何も起きない場所**に付ける。ここを叩いたらキーボードを閉じる。
    ///
    /// **画面全体には付けない。** 画面に付けると Form の中の Button や Toggle
    /// からタップを奪い、1タップ目が閉じるだけになる（#217 / #218）。
    /// 「キーボードが出ているときだけ受ける」でも、**コントロールを押すのに
    /// 2タップ要る**のは変わらず、使っていて煩わしかった（#229）。
    ///
    /// 合計や記録の一覧のように、**読むだけの行**に付けるのが正解。
    /// コントロールは常に1タップで効き、空いた場所では今までどおり閉じる。
    func dismissesKeyboardWhenTapped() -> some View {
        contentShape(Rectangle())
            .onTapGesture(perform: dismissKeyboard)
    }
}

extension View {
    /// キーボードの上に「完了」を置く。
    ///
    /// **数値キーボードには改行キーが無い。** 入力欄がぎっしり並ぶ画面では
    /// タップで閉じる余白も無いので、確実な逃げ道が要る。
    ///
    /// 欄の間を移動したい画面は、代わりに `keyboardFocusBar` を使う
    /// （ツールバーを2つ置くと両方出てしまう）。
    func keyboardDoneButton() -> some View {
        toolbar {
            ToolbarItemGroup(placement: .keyboard) {
                Spacer()
                Button("完了", action: dismissKeyboard)
            }
        }
    }

    /// キーボードの上に「◀ ▶ 完了」を置く。
    ///
    /// **数値キーボードは「次へ」を出せない。** 桁数が決まっていないので
    /// 自動で送ることもできない。バーから1タップで送れるようにする。
    /// - Parameter action: バーの右端に置く主操作。**キーボードに隠れる位置に
    ///   置いた操作は押せない**（#202 の UI テストが検出した）ので、ここに逃がす
    func keyboardFocusBar<F: Hashable>(
        focus: FocusState<F?>.Binding,
        order: [F],
        action: (title: String, run: () -> Void)? = nil
    ) -> some View {
        toolbar {
            ToolbarItemGroup(placement: .keyboard) {
                Button {
                    focus.wrappedValue = step(focus.wrappedValue, in: order, by: -1)
                } label: {
                    Image(systemName: "chevron.up")
                }
                .disabled(index(focus.wrappedValue, in: order) == 0)
                // UI テストから引くため（#226）
                .accessibilityIdentifier("keyboardPrev")

                Button {
                    focus.wrappedValue = step(focus.wrappedValue, in: order, by: 1)
                } label: {
                    Image(systemName: "chevron.down")
                }
                .disabled(index(focus.wrappedValue, in: order) == order.count - 1)
                .accessibilityIdentifier("keyboardNext")

                Spacer()
                if let action {
                    Button(action.title) {
                        focus.wrappedValue = nil
                        action.run()
                    }
                    .bold()
                    // **タブバーにも「記録」がある。** 名前だけでは引けないので
                    // UI テスト用に識別子を付ける（#202）
                    .accessibilityIdentifier("keyboardPrimaryAction")
                } else {
                    Button("完了") { focus.wrappedValue = nil }
                        .accessibilityIdentifier("keyboardDone")
                }
            }
        }
    }
}

private func index<F: Hashable>(_ current: F?, in order: [F]) -> Int? {
    current.flatMap { order.firstIndex(of: $0) }
}

/// 次（または前）の欄。端では動かさない。
private func step<F: Hashable>(_ current: F?, in order: [F], by delta: Int) -> F? {
    guard let i = index(current, in: order) else { return order.first }
    let next = i + delta
    guard order.indices.contains(next) else { return current }

    return order[next]
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
