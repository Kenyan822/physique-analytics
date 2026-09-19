package mealslot_test

import (
	"testing"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/mealslot"
)

func TestFromTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		at   string
		want openapi.MealSlot
	}{
		{"朝", "07:20", openapi.Breakfast},
		{"昼", "12:15", openapi.Lunch},
		{"夕", "19:40", openapi.Dinner},
		{"夜食", "23:30", openapi.Snack},

		// 境界。仕様の表をそのまま写す
		{"4:59 は間食", "04:59", openapi.Snack},
		{"5:00 から朝食", "05:00", openapi.Breakfast},
		{"10:59 まで朝食", "10:59", openapi.Breakfast},
		{"11:00 から昼食", "11:00", openapi.Lunch},
		{"15:59 まで昼食", "15:59", openapi.Lunch},
		{"16:00 から夕食", "16:00", openapi.Dinner},
		{"21:59 まで夕食", "21:59", openapi.Dinner},
		{"22:00 から間食", "22:00", openapi.Snack},

		// **深夜は間食。** Web の区切りだと 3:00 が朝食になっていた（#176）
		{"深夜3時は間食", "03:00", openapi.Snack},
		{"0:00 は間食", "00:00", openapi.Snack},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := mealslot.FromTime(tt.at)
			if err != nil {
				t.Fatalf("FromTime(%q) = error %v", tt.at, err)
			}
			if got != tt.want {
				t.Errorf("FromTime(%q) = %v, want %v", tt.at, got, tt.want)
			}
		})
	}
}

func TestFromTime_受け付けない値(t *testing.T) {
	t.Parallel()

	// **黙って既定値に落とさない。** 区分が静かにずれるより、弾いて気づく方がいい
	tests := []struct {
		name string
		at   string
	}{
		{"空", ""},
		{"時が範囲外", "24:00"},
		{"分が範囲外", "12:60"},
		{"区切りが無い", "1930"},
		{"秒まである", "19:40:00"},
		{"数字でない", "ごご"},
		{"桁が足りない", "9:40"},
		{"負", "-1:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := mealslot.FromTime(tt.at); err == nil {
				t.Errorf("FromTime(%q) がエラーにならない", tt.at)
			}
		})
	}
}

func TestValid(t *testing.T) {
	t.Parallel()

	if !mealslot.Valid("00:00") {
		t.Error(`Valid("00:00") = false`)
	}
	if mealslot.Valid("24:00") {
		t.Error(`Valid("24:00") = true`)
	}
}
