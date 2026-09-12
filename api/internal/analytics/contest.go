package analytics

import "time"

// 大会までのカウントダウンと必要ペース（要件 A-10 / P-04）。
// 仕様は docs/03-分析ロジック.md の週次レポート 1.5 節。

// SafePacePctPerWeek は減量ペースの安全域（体重に対する%/週）。
//
// **これを超えると LBM を失う。** 大会に間に合わせるために速めるのではなく、
// 減量開始を前倒しするか、ステージ体脂肪率の目標を緩める。
const SafePacePctPerWeek = 0.7

// ContestTarget は大会までに必要な減量。
type ContestTarget struct {
	// StageWeightKg は目標体脂肪率に到達したときの体重（LBM 維持前提）
	StageWeightKg float64
	// NeedLossKg は必要な減量。負なら既に下回っている
	NeedLossKg float64
	// PacePctPerWeek は残り週数から逆算した必要ペース
	PacePctPerWeek float64
	// TooFast は安全域を超えているか
	TooFast bool
}

// ContestPace は大会までに必要な減量ペースを返す。
//
// **LBM を維持する前提で計算する。** 実際には多少落ちるが、
// 「LBM を保ったまま到達するには何%/週 か」が判断の基準になる。
//
// 計算できない入力（残り週数が0以下など）では ok に false を返す。
func ContestPace(weightKg, bodyfatPct, targetBfPct, weeks float64) (ContestTarget, bool) {
	if weeks <= 0 || weightKg <= 0 || targetBfPct < 0 || targetBfPct >= 100 {
		return ContestTarget{}, false
	}

	lbm := weightKg * (1 - bodyfatPct/100)
	stage := lbm / (1 - targetBfPct/100)
	loss := weightKg - stage
	pace := loss / weightKg * 100 / weeks

	return ContestTarget{
		StageWeightKg:  stage,
		NeedLossKg:     loss,
		PacePctPerWeek: pace,
		TooFast:        pace > SafePacePctPerWeek,
	}, true
}

// WeeksUntil は asof から date までの週数を返す。過ぎていれば負。
//
// 端数を丸めない。残り 10日 を「1週」にすると、必要ペースが4割ずれる。
func WeeksUntil(asof, date time.Time) float64 {
	return date.Sub(asof).Hours() / 24 / daysPerWeek
}
