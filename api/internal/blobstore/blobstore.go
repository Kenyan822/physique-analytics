// Package blobstore は写真の置き場（要件 B-04 / B-05 / B-07）。
//
// **Cloudflare R2 に置く**（ADR-0008）。R2 は S3 互換なので、
// 署名付き URL の生成は SigV4 をそのまま使う。
//
// 公開URLは作らない。**API が所有者を検証してから短期の署名付きURLを
// 発行する**（docs/05-インフラ設計.md）。R2 は RLS が使えないため。
package blobstore

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// unsignedPayload は本文を署名対象にしないことを表す SigV4 の定数。
// 署名付きURLでは本文がまだ存在しないのでこれを使う。
const unsignedPayload = "UNSIGNED-PAYLOAD"

const (
	algorithm  = "AWS4-HMAC-SHA256"
	service    = "s3"
	timeFormat = "20060102T150405Z"
	dateFormat = "20060102"
	maxExpiry  = 7 * 24 * time.Hour
	minExpiry  = time.Second
)

// Config は R2 の接続設定。
//
// **どれかが空なら無効。** 未設定のまま起動しても、写真の機能だけが
// 使えない状態になる（課金は発生しない）。
type Config struct {
	AccountID       string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	// Region は R2 では常に auto
	Region string
	// Endpoint は差し替え用。空なら R2 の既定
	Endpoint string
}

// R2 は Cloudflare R2 への署名付き URL を発行する。
type R2 struct {
	cfg Config
}

// NewR2 は R2 を作る。
func NewR2(cfg Config) *R2 {
	if cfg.Region == "" {
		cfg.Region = "auto"
	}

	return &R2{cfg: cfg}
}

// Enabled は写真の機能が使える状態かを返す。
func (r *R2) Enabled() bool {
	return r.cfg.AccountID != "" && r.cfg.Bucket != "" &&
		r.cfg.AccessKeyID != "" && r.cfg.SecretAccessKey != ""
}

// PresignGet は取得用の署名付き URL を返す。
func (r *R2) PresignGet(key string, expiry time.Duration, now time.Time) (string, error) {
	return r.presign("GET", key, expiry, now)
}

// PresignPut はアップロード用の署名付き URL を返す。
//
// **API を経由させずに直接アップロードさせる。** 4MB の画像を
// Cloud Run に通すと、メモリもリクエスト時間も無駄になる。
func (r *R2) PresignPut(key string, expiry time.Duration, now time.Time) (string, error) {
	return r.presign("PUT", key, expiry, now)
}

// PhotoKey は保存するキーを組み立てる。
//
// 日付で並ぶ形にする。ストレージを直接見たときに時系列で読めた方が、
// 取り違えや欠損に気づきやすい。
func PhotoKey(date, pose, mimeType string) string {
	return fmt.Sprintf("photos/%s/%s%s", date, pose, extensionFor(mimeType))
}

func extensionFor(mimeType string) string {
	switch mimeType {
	case "image/png":
		return ".png"
	case "image/heic":
		return ".heic"
	default:
		return ".jpg"
	}
}

func (r *R2) presign(method, key string, expiry time.Duration, now time.Time) (string, error) {
	if !r.Enabled() {
		return "", errors.New(
			"写真の保存先が未設定。R2_ACCOUNT_ID / R2_BUCKET / R2_ACCESS_KEY_ID / R2_SECRET_ACCESS_KEY を設定する")
	}
	if expiry < minExpiry || expiry > maxExpiry {
		return "", fmt.Errorf("有効期限は %v 〜 %v にする", minExpiry, maxExpiry)
	}

	host := r.cfg.Endpoint
	if host == "" {
		host = r.cfg.AccountID + ".r2.cloudflarestorage.com"
	}

	path := "/" + r.cfg.Bucket + "/" + strings.TrimPrefix(key, "/")
	amzDate := now.UTC().Format(timeFormat)
	scope := strings.Join([]string{
		now.UTC().Format(dateFormat), r.cfg.Region, service, "aws4_request",
	}, "/")

	q := url.Values{}
	q.Set("X-Amz-Algorithm", algorithm)
	q.Set("X-Amz-Credential", r.cfg.AccessKeyID+"/"+scope)
	q.Set("X-Amz-Date", amzDate)
	q.Set("X-Amz-Expires", strconv.Itoa(int(expiry.Seconds())))
	q.Set("X-Amz-SignedHeaders", "host")

	canonical := strings.Join([]string{
		method,
		escapePath(path),
		q.Encode(),
		"host:" + host + "\n",
		"host",
		unsignedPayload,
	}, "\n")

	stringToSign := strings.Join([]string{
		algorithm, amzDate, scope, hashHex(canonical),
	}, "\n")

	q.Set("X-Amz-Signature", hex.EncodeToString(
		hmacSHA256(r.signingKey(now), stringToSign)))

	return "https://" + host + escapePath(path) + "?" + q.Encode(), nil
}

// signingKey は SigV4 の署名鍵を作る。日付・リージョン・サービスで段階的に導出する。
func (r *R2) signingKey(now time.Time) []byte {
	k := hmacSHA256([]byte("AWS4"+r.cfg.SecretAccessKey), now.UTC().Format(dateFormat))
	k = hmacSHA256(k, r.cfg.Region)
	k = hmacSHA256(k, service)

	return hmacSHA256(k, "aws4_request")
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))

	return h.Sum(nil)
}

func hashHex(s string) string {
	sum := sha256.Sum256([]byte(s))

	return hex.EncodeToString(sum[:])
}

// escapePath はパスをエスケープする。**`/` は残す**（S3 の正規化の規則）。
func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}

	return strings.Join(parts, "/")
}
