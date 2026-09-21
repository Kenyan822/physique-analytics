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
    /// キーボードが出ているか。**タップを受けるかどうかの判断にだけ使う**
    @State private var keyboardUp = false

    func body(content: Content) -> some View {
        content
            // .interactively にすると、指を下ろした量に追従して閉じる。
            // .immediately は少し触れただけで消えて、行を選びたいだけのときに邪魔
            .scrollDismissesKeyboard(.interactively)
            // **キーボードが出ているときだけ受ける。**
            //
            // 常に付けると Form の中の Button や Toggle からタップを奪う。
            // `simultaneousGesture` でも奪う。明示的な `buttonStyle` を持つ
            // ものだけが生き残るので、効くものと効かないものが混在して
            // 原因が見えにくくなる（#217 の Button、#218 の Toggle）。
            //
            // `including:` を切り替えるのは、`if` で付け外しすると
            // view の識別が変わってしまうため。
            // **キーボードが下りているときは `.subviews`** ＝ この gesture は
            // 働かず、下のコントロールがそのまま受ける
            .gesture(
                TapGesture().onEnded { dismissKeyboard() },
                including: keyboardUp ? .all : .subviews
            )
            .onReceive(NotificationCenter.default.publisher(
                for: UIResponder.keyboardWillShowNotification)) { _ in keyboardUp = true }
            .onReceive(NotificationCenter.default.publisher(
                for: UIResponder.keyboardWillHideNotification)) { _ in keyboardUp = false }
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

                Button {
                    focus.wrappedValue = step(focus.wrappedValue, in: order, by: 1)
                } label: {
                    Image(systemName: "chevron.down")
                }
                .disabled(index(focus.wrappedValue, in: order) == order.count - 1)

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
