// Package mealslot は食事の時刻から区分（朝食/昼食/夕食/間食）を導く。
//
// **導出をサーバに寄せている。** 以前はクライアントごとに実装があり、
// 境界がずれていた（Web は深夜3時を「朝食」、iOS は「間食」）。
// 1か所に置けば構造的に揃う。
package mealslot

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
)

// 区分の境界（時）。**開始時刻で持つ**。終了は次の区分の開始 - 1 分。
//
//	22:00 〜 4:59  間食
//	 5:00 〜 10:59 朝食
//	11:00 〜 15:59 昼食
//	16:00 〜 21:59 夕食
const (
	breakfastFrom = 5
	lunchFrom     = 11
	dinnerFrom    = 16
	snackFrom     = 22
)

// FromTime は "HH:MM" から区分を返す。
//
// **受け付けない値は既定値に落とさずエラーにする。** 黙って「朝食」にすると、
// 区分が静かにずれたまま記録が積まれる。
func FromTime(at string) (openapi.MealSlot, error) {
	h, _, err := parse(at)
	if err != nil {
		return "", err
	}

	switch {
	case h < breakfastFrom:
		// 0:00〜4:59。日付が変わってすぐは前日の続きなので間食
		return openapi.Snack, nil
	case h < lunchFrom:
		return openapi.Breakfast, nil
	case h < dinnerFrom:
		return openapi.Lunch, nil
	case h < snackFrom:
		return openapi.Dinner, nil
	default:
		return openapi.Snack, nil
	}
}

// Valid は "HH:MM" として解釈できるかを返す。
func Valid(at string) bool {
	_, _, err := parse(at)

	return err == nil
}

// parse は "HH:MM" を時と分に割る。
//
// **time.Parse を使わない。** "19:40:00" のような秒付きも通してしまい、
// DB の time 列に入れたときに API が返す形と食い違う。
func parse(at string) (hour, minute int, err error) {
	h, m, found := strings.Cut(at, ":")
	if !found || len(h) != 2 || len(m) != 2 {
		return 0, 0, fmt.Errorf("時刻は HH:MM で指定する: %q", at)
	}

	hour, err = twoDigits(h)
	if err != nil || hour > 23 {
		return 0, 0, fmt.Errorf("時が 00〜23 でない: %q", at)
	}

	minute, err = twoDigits(m)
	if err != nil || minute > 59 {
		return 0, 0, fmt.Errorf("分が 00〜59 でない: %q", at)
	}

	return hour, minute, nil
}

// twoDigits は2桁の数字を読む。
//
// **strconv.Atoi だけだと "+1" や " 9" が通る。** 先に数字かどうかを見る。
func twoDigits(s string) (int, error) {
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("数字でない: %q", s)
		}
	}

	return strconv.Atoi(s)
}
