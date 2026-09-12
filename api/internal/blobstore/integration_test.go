package blobstore_test

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Kenyan822/physique-analytics/api/internal/blobstore"
)

// TestPresign_S3互換サーバで通る は、署名が実際に受理されるかを確かめる。
//
// **自分のテストは自分の実装をなぞるだけ。** 署名が「それらしく見える」ことと
// 「サーバに受理される」ことは別なので、S3 互換の実装に投げて確かめる。
//
// MinIO を立てていなければ Skip する（CI では回らない）。
//
//	docker run -d -p 9000:9000 -e MINIO_ROOT_USER=TESTACCESSKEY \
//	  -e MINIO_ROOT_PASSWORD=testsecretkey123 quay.io/minio/minio server /data
func TestPresign_S3互換サーバで通る(t *testing.T) {
	t.Parallel()

	endpoint := os.Getenv("S3_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("S3_TEST_ENDPOINT が未設定（MinIO などを立てて指定する）")
	}

	s := blobstore.NewR2(blobstore.Config{
		Bucket:          envOr("S3_TEST_BUCKET", "physique-photos"),
		AccessKeyID:     envOr("S3_TEST_ACCESS_KEY", "TESTACCESSKEY"),
		SecretAccessKey: envOr("S3_TEST_SECRET_KEY", "testsecretkey123"),
		Region:          envOr("S3_TEST_REGION", "us-east-1"),
		Endpoint:        endpoint,
	})

	key := "photos/2035-03-01/front.jpg"
	body := []byte("fake-jpeg-bytes")

	putURL, err := s.PresignPut(key, 10*time.Minute, time.Now())
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, plain(putURL), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("リクエスト: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("PUT = %d: %s", res.StatusCode, raw)
	}

	getURL, err := s.PresignGet(key, 10*time.Minute, time.Now())
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}

	getReq, err := http.NewRequestWithContext(t.Context(), http.MethodGet, plain(getURL), nil)
	if err != nil {
		t.Fatalf("リクエスト: %v", err)
	}
	res2, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = res2.Body.Close() }()

	got, _ := io.ReadAll(res2.Body)
	if !bytes.Equal(got, body) {
		t.Errorf("中身が違う: %q", got)
	}

	// 署名を1文字変えたら弾かれる
	badReq, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		plain(getURL)[:len(plain(getURL))-1]+"0", nil)
	if err != nil {
		t.Fatalf("リクエスト: %v", err)
	}
	res3, err := http.DefaultClient.Do(badReq)
	if err != nil {
		t.Fatalf("GET（壊した署名）: %v", err)
	}
	defer func() { _ = res3.Body.Close() }()

	if res3.StatusCode < 400 {
		t.Errorf("壊した署名が通った: %d", res3.StatusCode)
	}
}

// plain は MinIO 向けに https を http にする。R2 は常に https。
func plain(u string) string {
	return "http" + strings.TrimPrefix(u, "https")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}
