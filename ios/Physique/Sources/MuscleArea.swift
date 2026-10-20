import Foundation

/// 部位の大分類（#288）。**表示を畳むためだけのもの。**
///
/// `MuscleGroup` は13ある。分析の部位別ボリューム（MEV/MRV）が13部位前提で、
/// **肩3分割・背中2分割は意図的**（docs/03-分析ロジック.md 分析4）。
/// 一括の 10-20 では判定できないので、まとめると分析が壊れる。
///
/// 画面に13の見出しが並ぶのは多いので、**表示だけ6つにする。**
/// 行には細かい部位を添えるので、どこに入るかは見えたまま。
enum MuscleArea: String, CaseIterable, Sendable {
    case chest, back, shoulders, arms, legs, core

    var label: String {
        switch self {
        case .chest: "胸"
        case .back: "背中"
        case .shoulders: "肩"
        case .arms: "腕"
        case .legs: "脚"
        case .core: "体幹"
        }
    }

    /// この大分類に入る細かい部位。**並びは画面に出る順**
    var groups: [MuscleGroup] {
        switch self {
        case .chest: [.chest]
        case .back: [.lats, .traps]
        case .shoulders: [.frontDelts, .sideDelts, .rearDelts]
        case .arms: [.biceps, .triceps]
        case .legs: [.quads, .hamstrings, .glutes, .calves]
        case .core: [.abs]
        }
    }

    /// どの大分類に入るか。**`MuscleGroup` を足したらここも足す**
    /// （`coversAll` のテストが落ちる）
    static func of(_ g: MuscleGroup) -> MuscleArea? {
        allCases.first { $0.groups.contains(g) }
    }
}

/// 種目を名前で絞る（#288）。
///
/// **49種目から探すなら、畳むより打って絞る方が速い。**
/// 部位名でも引けるようにする —— 「肩の種目」を探すときに種目名を覚えていない
enum ExerciseFilter {
    static func apply(_ list: [Exercise], query: String) -> [Exercise] {
        let q = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !q.isEmpty else { return list }

        return list.filter { e in
            e.name.localizedCaseInsensitiveContains(q)
                || e.muscleGroup.rawValue.contains(q)
                || (MuscleArea.of(e.muscleGroup)?.label.contains(q) ?? false)
        }
    }
}
