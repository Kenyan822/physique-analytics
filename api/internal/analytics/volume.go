package analytics

import "github.com/Kenyan822/physique-analytics/api/gen/openapi"

// VolumeRange は部位あたりの週間有効セット数の範囲。
//
//	MEV (Minimum Effective Volume) … これを下回ると刺激が足りない
//	MRV (Maximum Recoverable Volume) … これを超えると回復が追いつかない
type VolumeRange struct {
	MEV int
	MRV int
}

// volumeRanges は部位別の MEV/MRV。
//
// **一括の 10-20 では判定できない。** 肩を前部/中部/後部に分けているのは、
// プレス系で前部だけが伸びて中部・後部が置き去りになるのを検出するため。
// 前部は他の種目の巻き込みで勝手に積み上がるので上限が低い。
//
// config.example.json の volume_sets_per_muscle と同じ値。
// 個人の設定で上書きできるようにするのは、実データを扱う段階で考える。
var volumeRanges = map[openapi.MuscleGroup]VolumeRange{
	openapi.Chest:      {10, 20},
	openapi.Lats:       {10, 20},
	openapi.Traps:      {8, 18},
	openapi.FrontDelts: {4, 12},
	openapi.SideDelts:  {8, 14},
	openapi.RearDelts:  {4, 14},
	openapi.Biceps:     {8, 16},
	openapi.Triceps:    {8, 16},
	openapi.Quads:      {10, 20},
	openapi.Hamstrings: {8, 16},
	openapi.Glutes:     {6, 14},
	openapi.Calves:     {8, 16},
	openapi.Abs:        {4, 16},
}

// defaultVolumeRange は未知の部位に使う。
var defaultVolumeRange = VolumeRange{MEV: 10, MRV: 20}

// VolumeRangeFor は部位の MEV/MRV を返す。
func VolumeRangeFor(mg openapi.MuscleGroup) VolumeRange {
	if r, ok := volumeRanges[mg]; ok {
		return r
	}

	return defaultVolumeRange
}

// VolumeVerdict はセット数の判定。
type VolumeVerdict string

const (
	// VolumeBelowMEV は刺激不足。増やす
	VolumeBelowMEV VolumeVerdict = "MEV以下"
	// VolumeOK は適正
	VolumeOK VolumeVerdict = "適正"
	// VolumeAboveMRV は回復不足。減らす
	VolumeAboveMRV VolumeVerdict = "MRV超"
)

// JudgeVolume は週間セット数を MEV/MRV と突き合わせて判定する。
func JudgeVolume(mg openapi.MuscleGroup, sets int) (VolumeVerdict, VolumeRange) {
	r := VolumeRangeFor(mg)

	switch {
	case sets < r.MEV:
		return VolumeBelowMEV, r
	case sets > r.MRV:
		return VolumeAboveMRV, r
	}

	return VolumeOK, r
}
