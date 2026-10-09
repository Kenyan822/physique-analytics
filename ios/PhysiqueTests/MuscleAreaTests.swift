import Foundation
import Testing

@testable import PhysiqueCore

@Suite("部位を6つに畳む")
struct MuscleAreaTests {
    @Test("**13部位が6つに畳まれる**")
    func groups() {
        #expect(MuscleArea.allCases.count == 6)
        #expect(MuscleArea.allCases.map(\.label)
                == ["胸", "背中", "肩", "腕", "脚", "体幹"])
    }

    @Test("どの部位も必ずどこかに入る")
    func coversAll() {
        for g in MuscleGroup.allCases {
            #expect(MuscleArea.of(g) != nil)
        }
    }

    @Test("中身が正しい")
    func members() {
        #expect(MuscleArea.of(.lats) == .back)
        #expect(MuscleArea.of(.traps) == .back)
        #expect(MuscleArea.of(.sideDelts) == .shoulders)
        #expect(MuscleArea.of(.biceps) == .arms)
        #expect(MuscleArea.of(.calves) == .legs)
        #expect(MuscleArea.of(.abs) == .core)
    }

    @Test("**細かい部位は失わない**（分析が13部位前提）")
    func keepsFineGroups() {
        // 畳むのは表示だけ。enum の値は変えない
        #expect(MuscleGroup.allCases.count == 13)
        #expect(MuscleArea.shoulders.groups == [.frontDelts, .sideDelts, .rearDelts])
    }
}

@Suite("種目を名前で絞る")
struct ExerciseSearchTests {
    private let list = [
        Exercise(id: UUID(), name: "ベンチプレス", muscleGroup: .chest),
        Exercise(id: UUID(), name: "インクラインベンチプレス", muscleGroup: .chest),
        Exercise(id: UUID(), name: "ラットプルダウン", muscleGroup: .lats),
        Exercise(id: UUID(), name: "サイドレイズ", muscleGroup: .sideDelts),
    ]

    @Test("空なら全部")
    func empty() {
        #expect(ExerciseFilter.apply(list, query: "").count == 4)
        #expect(ExerciseFilter.apply(list, query: "   ").count == 4)
    }

    @Test("部分一致で絞る")
    func partial() {
        #expect(ExerciseFilter.apply(list, query: "ベンチ").map(\.name)
                == ["ベンチプレス", "インクラインベンチプレス"])
    }

    @Test("**部位名でも引ける**")
    func byMuscle() {
        #expect(ExerciseFilter.apply(list, query: "広背筋").map(\.name) == ["ラットプルダウン"])
    }

    @Test("大分類でも引ける")
    func byArea() {
        #expect(ExerciseFilter.apply(list, query: "肩").map(\.name) == ["サイドレイズ"])
    }

    @Test("一致しなければ空")
    func noMatch() {
        #expect(ExerciseFilter.apply(list, query: "スクワット").isEmpty)
    }
}
