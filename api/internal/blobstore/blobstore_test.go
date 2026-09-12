package blobstore_test

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Kenyan822/physique-analytics/api/internal/blobstore"
)

func config() blobstore.Config {
	return blobstore.Config{
		AccountID:       "acct",
		Bucket:          "physique-photos",
		AccessKeyID:     "AKIAEXAMPLE",
		SecretAccessKey: "secret",
		Region:          "auto",
	}
}

// fixedClock はテストを時刻に依存させない。
func fixedClock() time.Time {
	return time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
}

func TestPresignGet_署名付きURLを作る(t *testing.T) {
	t.Parallel()

	s := blobstore.NewR2(config())

	raw, err := s.PresignGet("photos/2026-10-31/front.jpg", 15*time.Minute, fixedClock())
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("URL: %v", err)
	}

	// R2 の S3 互換エンドポイント
	if u.Host != "acct.r2.cloudflarestorage.com" {
		t.Errorf("Host = %q", u.Host)
	}
	if u.Path != "/physique-photos/photos/2026-10-31/front.jpg" {
		t.Errorf("Path = %q", u.Path)
	}

	q := u.Query()
	if q.Get("X-Amz-Algorithm") != "AWS4-HMAC-SHA256" {
		t.Errorf("Algorithm = %q", q.Get("X-Amz-Algorithm"))
	}
	if q.Get("X-Amz-Expires") != "900" {
		t.Errorf("Expires = %q, want 900", q.Get("X-Amz-Expires"))
	}
	if q.Get("X-Amz-Signature") == "" {
		t.Error("署名が無い")
	}
	// **公開URLを作らない**（ADR-0008）。資格情報がクエリに入る
	if !strings.Contains(q.Get("X-Amz-Credential"), "AKIAEXAMPLE") {
		t.Errorf("Credential = %q", q.Get("X-Amz-Credential"))
	}
}

func TestPresign_署名は入力で変わる(t *testing.T) {
	t.Parallel()

	s := blobstore.NewR2(config())
	at := fixedClock()

	a, _ := s.PresignGet("photos/a.jpg", time.Minute, at)
	b, _ := s.PresignGet("photos/b.jpg", time.Minute, at)
	if signature(a) == signature(b) {
		t.Error("キーが違うのに署名が同じ")
	}

	c, _ := s.PresignGet("photos/a.jpg", 2*time.Minute, at)
	if signature(a) == signature(c) {
		t.Error("有効期限が違うのに署名が同じ")
	}
}

func TestPresign_同じ入力なら同じ署名(t *testing.T) {
	t.Parallel()

	s := blobstore.NewR2(config())
	at := fixedClock()

	a, _ := s.PresignGet("photos/a.jpg", time.Minute, at)
	b, _ := s.PresignGet("photos/a.jpg", time.Minute, at)
	if a != b {
		t.Error("同じ入力で URL が変わる")
	}
}

func TestPresignPut_アップロード用(t *testing.T) {
	t.Parallel()

	s := blobstore.NewR2(config())

	raw, err := s.PresignPut("photos/x.jpg", time.Minute, fixedClock())
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}
	// GET と PUT で署名が変わる（メソッドが署名対象に入る）
	get, _ := s.PresignGet("photos/x.jpg", time.Minute, fixedClock())
	if signature(raw) == signature(get) {
		t.Error("GET と PUT の署名が同じ")
	}
}

func TestPresign_未設定ならエラー(t *testing.T) {
	t.Parallel()

	s := blobstore.NewR2(blobstore.Config{})

	if _, err := s.PresignGet("x", time.Minute, fixedClock()); err == nil {
		t.Fatal("エラーにならない")
	}
}

func TestEnabled(t *testing.T) {
	t.Parallel()

	if blobstore.NewR2(blobstore.Config{}).Enabled() {
		t.Error("未設定なのに有効")
	}
	if !blobstore.NewR2(config()).Enabled() {
		t.Error("設定済みなのに無効")
	}
}

func TestPhotoKey(t *testing.T) {
	t.Parallel()

	// 日付で並ぶキーにする。ストレージを直接見たときに時系列で読める
	got := blobstore.PhotoKey("2026-10-31", "front", "image/jpeg")
	if got != "photos/2026-10-31/front.jpg" {
		t.Errorf("PhotoKey = %q", got)
	}

	if got := blobstore.PhotoKey("2026-10-31", "back", "image/heic"); got != "photos/2026-10-31/back.heic" {
		t.Errorf("PhotoKey(heic) = %q", got)
	}
}

func signature(raw string) string {
	u, _ := url.Parse(raw)

	return u.Query().Get("X-Amz-Signature")
}
