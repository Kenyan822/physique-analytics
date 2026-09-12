package auth_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
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

	b64 := func(i *big.Int) string {
		// P-256 は 32 バイト固定。左ゼロ埋めしないと鍵が壊れる
		buf := make([]byte, 32)
		i.FillBytes(buf)
		return base64.RawURLEncoding.EncodeToString(buf)
	}

	body, err := json.Marshal(map[string]any{
		"keys": []map[string]string{{
			"kty": "EC", "crv": "P-256", "alg": "ES256", "use": "sig",
			"kid": testKID,
			"x":   b64(pub.X), "y": b64(pub.Y),
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

func newVerifier(t *testing.T) (*auth.Verifier, *ecdsa.PrivateKey) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("鍵を作れない: %v", err)
	}
	srv := jwksServer(t, &key.PublicKey)

	return auth.NewVerifier(srv.URL), key
}

func validClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"sub": "11111111-1111-1111-1111-111111111111",
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
