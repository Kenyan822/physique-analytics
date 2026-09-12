// Package timeutil はこのアプリが扱うタイムゾーンを一箇所に固定する。
package timeutil

import "time"

// JST は日本標準時。ADR-0013 により、このアプリの時刻表現はすべて JST に固定する。
//
// time.LoadLocation("Asia/Tokyo") を使わないのは、distroless イメージに tzdata が
// 含まれておらず実行時に失敗するため。日本は夏時間を採用していないので固定オフセットで足りる。
var JST = time.FixedZone("JST", 9*60*60)

// Now は JST の現在時刻を返す。サーバの TZ 設定に依存させないため、
// アプリ内で time.Now() を直接呼ばずこれを使う。
func Now() time.Time { return time.Now().In(JST) }
