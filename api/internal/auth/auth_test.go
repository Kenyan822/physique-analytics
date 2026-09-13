package auth_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/Kenyan822/physique-analytics/api/internal/auth"
)

const testKID = "test-key-1"

// jwksServer は Supabase の JWKS エンドポイントを模した HTTP サーバを立てる。
// 本物に繋ぐとテストがネットワークとプロジェクトの状態に依存する。
func jwksServer(t *testing.T, pub *ecdsa.PublicKey) *httptest.Server {
	t.Helper()

	// 非圧縮形式（0x04 || X || Y）から座標を取り出す。
	// PublicKey.X / .Y は Go 1.26 で非推奨
	raw, err := pub.Bytes()
	if err != nil {
		t.Fatalf("公開鍵を符号化できない: %v", err)
	}
	if len(raw) != 1+2*32 {
		t.Fatalf("公開鍵の長さが想定外: %d", len(raw))
	}
	b64 := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

	body, err := json.Marshal(map[string]any{
		"keys": []map[string]string{{
			"kty": "EC", "crv": "P-256", "alg": "ES256", "use": "sig",
			"kid": testKID,
			"x":   b64(raw[1:33]), "y": b64(raw[33:]),
		}},
	})
	if err != nil {
		t.Fatalf("JWKS を組み立てられない: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	return srv
}

func signToken(t *testing.T, key *ecdsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()

	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tok.Header["kid"] = testKID
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("署名できない: %v", err)
	}

	return s
}

// newVerifier は validClaims の sub を許可した Verifier を返す。
// 許可リストそのものを試すテストは newVerifierAllowing を使う
func newVerifier(t *testing.T) (*auth.Verifier, *ecdsa.PrivateKey) {
	t.Helper()

	return newVerifierAllowing(t, testSub)
}

const testSub = "11111111-1111-1111-1111-111111111111"

func validClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"sub": testSub,
		"aud": "authenticated",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
}

func TestVerify_正しいトークンを受け入れる(t *testing.T) {
	t.Parallel()

	v, key := newVerifier(t)
	got, err := v.Verify(t.Context(), signToken(t, key, validClaims()))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.UserID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("userID = %q", got.UserID)
	}
}

func TestVerify_期限切れは弾く(t *testing.T) {
	t.Parallel()

	v, key := newVerifier(t)
	c := validClaims()
	c["exp"] = time.Now().Add(-time.Minute).Unix()

	if _, err := v.Verify(t.Context(), signToken(t, key, c)); err == nil {
		t.Fatal("エラーを期待したが nil")
	}
}

// 別の鍵で署名されたトークンを通すと、誰でも認証を突破できる
func TestVerify_別の鍵で署名されたものは弾く(t *testing.T) {
	t.Parallel()

	v, _ := newVerifier(t)
	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("鍵を作れない: %v", err)
	}

	if _, err := v.Verify(t.Context(), signToken(t, other, validClaims())); err == nil {
		t.Fatal("エラーを期待したが nil")
	}
}

// alg: none や HS256 への差し替えは古典的な攻撃手法
func TestVerify_algを差し替えたものは弾く(t *testing.T) {
	t.Parallel()

	v, _ := newVerifier(t)

	// HS256 で「公開鍵を鍵として」署名する。alg を見ずに検証すると通ってしまう
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims())
	tok.Header["kid"] = testKID
	s, err := tok.SignedString([]byte("whatever"))
	if err != nil {
		t.Fatalf("署名できない: %v", err)
	}

	if _, err := v.Verify(t.Context(), s); err == nil {
		t.Fatal("HS256 のトークンが通った。alg の検証が効いていない")
	}
}

func TestVerify_subが無ければ弾く(t *testing.T) {
	t.Parallel()

	v, key := newVerifier(t)
	c := validClaims()
	delete(c, "sub")

	if _, err := v.Verify(t.Context(), signToken(t, key, c)); err == nil {
		t.Fatal("エラーを期待したが nil")
	}
}

func TestVerify_壊れたトークンは弾く(t *testing.T) {
	t.Parallel()

	v, _ := newVerifier(t)
	for _, s := range []string{"", "not-a-jwt", "a.b.c"} {
		if _, err := v.Verify(t.Context(), s); err == nil {
			t.Errorf("%q が通った", s)
		}
	}
}

// --- ミドルウェア ---

func TestMiddleware_認証が要らないパスは素通しする(t *testing.T) {
	t.Parallel()

	v, _ := newVerifier(t)
	h := auth.Middleware(v, "/health")(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestMiddleware_トークンが無ければ401(t *testing.T) {
	t.Parallel()

	v, _ := newVerifier(t)
	h := auth.Middleware(v, "/health")(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/exercises", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("Content-Type = %q, want problem+json", got)
	}
	// RFC 6750: 401 には WWW-Authenticate を付ける
	if got := rec.Header().Get("WWW-Authenticate"); got == "" {
		t.Error("WWW-Authenticate が無い")
	}
}

func TestMiddleware_正しいトークンなら通す(t *testing.T) {
	t.Parallel()

	v, key := newVerifier(t)
	h := auth.Middleware(v, "/health")(okHandler())

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/exercises", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, key, validClaims()))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestMiddleware_ユーザーIDをcontextに載せる(t *testing.T) {
	t.Parallel()

	v, key := newVerifier(t)

	var got string
	h := auth.Middleware(v, "/health")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, _ = auth.UserIDFrom(r.Context())
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/exercises", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, key, validClaims()))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("context の userID = %q", got)
	}
}

func TestMiddleware_Bearer以外のスキームは401(t *testing.T) {
	t.Parallel()

	v, key := newVerifier(t)
	h := auth.Middleware(v, "/health")(okHandler())

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/exercises", nil)
	req.Header.Set("Authorization", "Basic "+signToken(t, key, validClaims()))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// --- 許可リスト（#142） ---
//
// JWT の検証は「Supabase が発行したか」しか見ない。サインアップが開いていれば
// 他人も有効なトークンを持てるので、**誰の sub かまで見る**。

const otherSub = "22222222-2222-2222-2222-222222222222"

func newVerifierAllowing(t *testing.T, allowed ...string) (*auth.Verifier, *ecdsa.PrivateKey) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("鍵を作れない: %v", err)
	}
	srv := jwksServer(t, &key.PublicKey)

	return auth.NewVerifier(srv.URL, allowed...), key
}

func TestVerify_許可リストにあるsubは通る(t *testing.T) {
	t.Parallel()

	v, key := newVerifierAllowing(t, testSub)
	if _, err := v.Verify(t.Context(), signToken(t, key, validClaims())); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerify_許可リストに無いsubは弾く(t *testing.T) {
	t.Parallel()

	// 署名も期限も正しいが、別人のトークン
	v, key := newVerifierAllowing(t, otherSub)
	_, err := v.Verify(t.Context(), signToken(t, key, validClaims()))

	if !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestVerify_許可リストが空なら誰も通さない(t *testing.T) {
	t.Parallel()

	// **fail-closed**（#152）。署名も期限も正しいトークンでも弾く。
	// 「設定するまで開いている」は「ログインするまで開いている」と同じで、
	// 順序が逆になる
	v, key := newVerifierAllowing(t)
	_, err := v.Verify(t.Context(), signToken(t, key, validClaims()))

	if !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestMiddleware_許可リストが空でも公開パスは通る(t *testing.T) {
	t.Parallel()

	// **ここが通らないとデプロイが落ちる。** スモークテストと keepalive が /health を叩く
	v, _ := newVerifierAllowing(t)
	h := auth.Middleware(v, "/health")(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestMiddleware_許可リストが空なら401(t *testing.T) {
	t.Parallel()

	v, key := newVerifierAllowing(t)
	h := auth.Middleware(v, "/health")(okHandler())

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/exercises", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, key, validClaims()))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestVerify_許可リストの空白と空要素は無視する(t *testing.T) {
	t.Parallel()

	// 環境変数に "a, b," と書かれても意図どおりに動くこと。
	// 空要素をそのまま入れると sub が空のトークンを通しうる
	v, key := newVerifierAllowing(t, "  "+testSub+"  ", "", "   ")
	if _, err := v.Verify(t.Context(), signToken(t, key, validClaims())); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestMiddleware_許可リストに無いsubは401(t *testing.T) {
	t.Parallel()

	v, key := newVerifierAllowing(t, otherSub)
	h := auth.Middleware(v, "/health")(okHandler())

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/exercises", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, key, validClaims()))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	// **理由を細かく返さない。** 「その sub は許可されていない」と返すと、
	// 誰が許可されているかを総当たりで探れる
	if strings.Contains(rec.Body.String(), otherSub) {
		t.Errorf("body に sub が漏れている: %s", rec.Body.String())
	}
}
