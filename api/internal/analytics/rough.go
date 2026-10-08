package analytics

import "math"

// ざっくり入力（要件 N-01）の既定比。kcal 比で P 20% / F 30% / C 50%。
//
// **飲み会に寄せた比率を決め打ちしない。** 焼き鳥中心と居酒屋コースでは全く違うので、
// どう決めても外れる。手で直せることの方が大事（#252）。
const (
	roughProteinShare = 0.20
	roughCarbShare    = 0.50
)

// RoughMacros は kcal だけから P・F・C（g）を按分する。
//
// 小数1桁に丸める（保存先が numeric(6,1)）。**丸めが kcal に効かないようにする**:
// 3つを別々に丸めると、PFC から kcal を出し直したとき最大 0.85 kcal ずれ、
// 「1000 と入れたのに 1001 と出る」ことがある。そこで P と C を丸め、
// **F は残りの kcal から逆算して丸める**。F の丸め誤差は最大 0.05g × 9 = 0.45 kcal で、
// 四捨五入すると必ず元の kcal に戻る。代償は F が比率から最大 0.1g 程度動くこと。
func RoughMacros(kcal int) (proteinG, fatG, carbG float64) {
	k := float64(kcal)

	proteinG = round1(k * roughProteinShare / kcalPerGProtein)
	carbG = round1(k * roughCarbShare / kcalPerGCarb)
	fatG = math.Max(0, round1((k-proteinG*kcalPerGProtein-carbG*kcalPerGCarb)/kcalPerGFat))

	return proteinG, fatG, carbG
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
