import Foundation

/// UI テストの待ち時間。
///
/// **CI のランナーは手元よりずっと遅い。** 5秒は手元では十分でも CI では足りず、
/// 製品に問題が無いのに赤くなる。実際 `MealInputTests` が2回続けて別々の箇所で
/// 落ちた（#253）。
///
/// 伸ばしても**assertion は弱くならない**。要素が本当に出ないなら、やはり落ちる。
/// 落ちるまでの時間が延びるだけ。
enum UITimeout {
    /// 画面の中の要素が出るまで
    static let normal: TimeInterval = 15

    /// アプリの起動やタブの切り替えを挟むとき
    static let slow: TimeInterval = 30
}
