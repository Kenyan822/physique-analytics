import Foundation
import Testing

@testable import PhysiqueCore

@Suite("周囲長の差分")
struct MeasurementDiffTests {
    @Test("増えたら + を付ける")
    func plus() {
        #expect(formatDiff(current: 86.5, previous: 86) == "+0.5")
    }

    @Test("減ったら − を付ける")
    func minus() {
        #expect(formatDiff(current: 85, previous: 86) == "−1")
    }

    @Test("同じなら出さない")
    func same() {
        #expect(formatDiff(current: 86, previous: 86) == nil)
    }

    @Test("前回が無ければ出さない")
    func noPrevious() {
        #expect(formatDiff(current: 86, previous: nil) == nil)
    }

    @Test("今回が無ければ出さない")
    func noCurrent() {
        #expect(formatDiff(current: nil, previous: 86) == nil)
    }

    // 39.1 - 39 は 0.10000000000000142 になる
    @Test("浮動小数点の誤差を出さない")
    func noFloatNoise() {
        #expect(formatDiff(current: 39.1, previous: 39) == "+0.1")
    }
}

@Suite("数値の読み取り")
struct ParseNumberTests {
    @Test("小数を読む")
    func decimal() {
        #expect(parseNumber("62.5") == 62.5)
    }

    // 日本語入力のまま数字を打つと全角になる
    @Test("全角数字を読む")
    func fullWidth() {
        #expect(parseNumber("６２．５") == 62.5)
    }

    @Test("前後の空白を無視する")
    func trims() {
        #expect(parseNumber(" 8 ") == 8)
    }

    @Test("空文字は未入力")
    func empty() {
        #expect(parseNumber("") == nil)
    }

    @Test("数値でなければ未入力")
    func notANumber() {
        #expect(parseNumber("abc") == nil)
    }

    @Test("マイナスは読まない")
    func negative() {
        #expect(parseNumber("-3") == nil)
    }
}

@Suite("BodyModel")
@MainActor
struct BodyModelTests {
    /// 直近の周囲長と今日の記録を返す偽 transport。
    private func transport(latest: String, daily: String) -> FakeTransport {
        let t = FakeTransport()
        t.responses = [(Data(daily.utf8), 200), (Data(latest.utf8), 200)]

        return t
    }

    @Test("前回値を差分の基準にする")
    func previousBecomesBaseline() async throws {
        let latest = """
        {"measurement":{"id":"11111111-1111-1111-1111-111111111111","date":"2026-10-25",
        "neckCm":38.2,"waistNavelCm":81.0,"createdAt":"2026-10-25T00:00:00+09:00",
        "updatedAt":"2026-10-25T00:00:00+09:00"}}
        """
        let model = BodyModel(api: APIClient(
            baseURL: URL(string: "http://x")!,
            transport: transport(latest: latest, daily: "{\"items\":[]}")
        ))

        await model.load(date: "2026-11-01")

        #expect(model.previousMeasurement?.neckCm == 38.2)
        // **今日の分はまだ無い。** 前回値は初期値ではなく比較対象
        #expect(model.parts[.neck] == nil)
    }

    @Test("今日すでに測っていれば読み込む")
    func todayLoaded() async throws {
        let latest = """
        {"measurement":{"id":"11111111-1111-1111-1111-111111111111","date":"2026-11-01",
        "neckCm":39.5,"createdAt":"2026-11-01T00:00:00+09:00",
        "updatedAt":"2026-11-01T00:00:00+09:00"}}
        """
        let model = BodyModel(api: APIClient(
            baseURL: URL(string: "http://x")!,
            transport: transport(latest: latest, daily: "{\"items\":[]}")
        ))

        await model.load(date: "2026-11-01")

        #expect(model.parts[.neck] == 39.5)
        // 今日の分が前回値になると、差分が常に 0 になる
        #expect(model.previousMeasurement == nil)
    }

    @Test("疲労度は同じ値を押すと外れる")
    func fatigueToggles() {
        let model = BodyModel(api: APIClient(
            baseURL: URL(string: "http://x")!, transport: FakeTransport()
        ))

        model.setFatigue(3)
        #expect(model.fatigue == 3)

        model.setFatigue(3)
        #expect(model.fatigue == nil)
    }

    @Test("触った項目だけ送る")
    func sendsOnlyTouched() {
        let model = BodyModel(api: APIClient(
            baseURL: URL(string: "http://x")!, transport: FakeTransport()
        ))
        model.parts[.neck] = 39.5

        let input = model.measurementInput(date: "2026-11-01")

        #expect(input.neckCm == 39.5)
        // **送らない項目は API 側で「変更しない」になる**
        #expect(input.shoulderCm == nil)
    }
}

@Suite("日付をずらす")
struct ShiftDaysTests {
    @Test("月をまたぐ")
    func acrossMonth() {
        #expect(shiftDays("2026-10-31", 1) == "2026-11-01")
    }

    @Test("年をまたぐ")
    func acrossYear() {
        #expect(shiftDays("2026-12-31", 1) == "2027-01-01")
    }

    @Test("戻せる")
    func backwards() {
        #expect(shiftDays("2026-01-01", -1) == "2025-12-31")
    }

    // **JST で計算する。** UTC で計算すると日付の境界が JST.dateString とずれる
    @Test("30日前")
    func thirtyDaysAgo() {
        #expect(shiftDays("2026-11-01", -30) == "2026-10-02")
    }
}

@Suite("除脂肪体重")
struct LeanBodyMassTests {
    @Test("体重から脂肪を引く")
    func calculates() {
        // 80kg・体脂肪率 20% → 64kg
        #expect(leanBodyMass(weightKg: 80, bodyfatPct: 20) == 64)
    }

    @Test("小数が出ても丸めない")
    func keepsPrecision() {
        let got = leanBodyMass(weightKg: 70, bodyfatPct: 15.5)
        #expect(got != nil)
        // 70 × 0.845 = 59.15
        #expect(abs((got ?? 0) - 59.15) < 0.001)
    }

    @Test("どちらかが無ければ出さない")
    func needsBoth() {
        // **0 と「測っていない」を混ぜない。** 片方だけでは計算できない
        #expect(leanBodyMass(weightKg: 80, bodyfatPct: nil) == nil)
        #expect(leanBodyMass(weightKg: nil, bodyfatPct: 20) == nil)
        #expect(leanBodyMass(weightKg: nil, bodyfatPct: nil) == nil)
    }

    @Test("ありえない体脂肪率は出さない")
    func rejectsOutOfRange() {
        // 体組成計が壊れた値を返すことがある。**負の除脂肪体重を作らない**
        #expect(leanBodyMass(weightKg: 80, bodyfatPct: 120) == nil)
        #expect(leanBodyMass(weightKg: 80, bodyfatPct: -1) == nil)
    }
}
