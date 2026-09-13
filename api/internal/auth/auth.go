// Package auth は Supabase が発行した JWT を検証する。
//
// Supabase は **非対称鍵（ES256）** で署名し、公開鍵を JWKS として
// 公開している。共有シークレットが要らないので、検証側に秘密情報を
// 置かなくて済む。
//
//	https://<project-ref>.supabase.co/auth/v1/.well-known/jwks.json
package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// jwksTTL は JWKS のキャッシュ保持時間。
// 鍵はめったに変わらないが、失効したときに追随できる程度には短くする。
const jwksTTL = time.Hour

// fetchTimeout は JWKS 取得のタイムアウト。
const fetchTimeout = 5 * time.Second

// Claims は検証済みトークンから取り出した情報。
type Claims struct {
	// UserID は Supabase の auth.users.id（JWT の sub）
	UserID string
}

// Verifier は JWT を検証する。JWKS をキャッシュする。
type Verifier struct {
	jwksURL string
	client  *http.Client

	// allowed は通す sub の集合。**空なら誰も通さない**（#152）
	allowed map[string]bool

	mu        sync.RWMutex
	keys      map[string]*ecdsa.PublicKey
	fetchedAt time.Time
}

// NewVerifier は Verifier を作る。
//
// allowedUserIDs に渡した sub のトークンしか通さない。
// 署名の検証は「Supabase が発行したか」しか見ないので、サインアップが
// 開いていれば他人も有効なトークンを持てる（#51 / #142）。
//
// **空なら誰も通さない**（#152）。「設定するまで開いている」は
// 「ログインするまで開いている」と同じで、順序が逆になる。
// 公開パス（/health）は Middleware 側で素通しするので、未設定でも
// デプロイのスモークテストと keepalive は通る。
func NewVerifier(jwksURL string, allowedUserIDs ...string) *Verifier {
	allowed := make(map[string]bool, len(allowedUserIDs))
	for _, id := range allowedUserIDs {
		// 環境変数に "a, b," と書かれても意図どおりに動かす。
		// 空要素を入れると sub が空のトークンを通しうる
		if t := strings.TrimSpace(id); t != "" {
			allowed[t] = true
		}
	}

	return &Verifier{
		jwksURL: jwksURL,
		client:  &http.Client{Timeout: fetchTimeout},
		allowed: allowed,
		keys:    map[string]*ecdsa.PublicKey{},
	}
}

// Allowlisted は許可リストが設定されているかを返す。
// 起動時の警告に使う（未設定だと、公開パス以外はすべて 401 になる）。
func (v *Verifier) Allowlisted() bool { return len(v.allowed) > 0 }

// ErrUnauthorized はトークンが受け入れられないことを表す。
//
// **理由を細かく外に返さない。** 「署名が違う」「期限切れ」「kid が無い」を
// 区別して返すと、総当たりの手がかりになる。詳細はログにだけ残す。
var ErrUnauthorized = errors.New("認証できない")

// Verify はトークンを検証し、クレームを返す。
func (v *Verifier) Verify(ctx context.Context, token string) (Claims, error) {
	claims, err := v.parse(ctx, token)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %w", ErrUnauthorized, err)
	}
	if !v.allowed[claims.UserID] {
		// **sub を返さない。** 誰が許可されているかを総当たりで探れてしまう
		return Claims{}, fmt.Errorf("%w: 許可されていない利用者", ErrUnauthorized)
	}

	return claims, nil
}

// keyFor は kid に対応する公開鍵を返す。無ければ JWKS を取り直す。
func (v *Verifier) keyFor(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	v.mu.RLock()
	key, ok := v.keys[kid]
	fresh := time.Since(v.fetchedAt) < jwksTTL
	v.mu.RUnlock()

	if ok && fresh {
		return key, nil
	}

	if err := v.refresh(ctx); err != nil {
		// 取り直しに失敗しても、手元に鍵があれば使う。
		// JWKS の一時的な障害で全リクエストが落ちるのを避ける
		if ok {
			return key, nil
		}
		return nil, err
	}

	v.mu.RLock()
	key, ok = v.keys[kid]
	v.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("kid %q に対応する鍵が無い", kid)
	}

	return key, nil
}

func (v *Verifier) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("JWKS のリクエストを作れない: %w", err)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("JWKS を取得できない: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS が %d を返した", resp.StatusCode)
	}

	var doc struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("JWKS を解釈できない: %w", err)
	}

	keys := make(map[string]*ecdsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		pub, err := k.publicKey()
		if err != nil {
			// 1つ壊れていても他が使えるなら続ける
			continue
		}
		keys[k.Kid] = pub
	}
	if len(keys) == 0 {
		return errors.New("JWKS に使える鍵が無い")
	}

	v.mu.Lock()
	v.keys = keys
	v.fetchedAt = time.Now()
	v.mu.Unlock()

	return nil
}

// jwk は JSON Web Key のうち、ES256 の検証に要る項目だけ。
type jwk struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Kid string `json:"kid"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// p256CoordLen は P-256 の座標1つ分のバイト数。
const p256CoordLen = 32

func (k jwk) publicKey() (*ecdsa.PublicKey, error) {
	if k.Kty != "EC" || k.Crv != "P-256" {
		return nil, fmt.Errorf("対応していない鍵: kty=%q crv=%q", k.Kty, k.Crv)
	}

	x, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil {
		return nil, fmt.Errorf("x を解釈できない: %w", err)
	}
	y, err := base64.RawURLEncoding.DecodeString(k.Y)
	if err != nil {
		return nil, fmt.Errorf("y を解釈できない: %w", err)
	}
	if len(x) != p256CoordLen || len(y) != p256CoordLen {
		return nil, fmt.Errorf("座標の長さが不正: x=%d y=%d", len(x), len(y))
	}

	// ecdsa.PublicKey の X / Y は Go 1.26 で非推奨になった。
	// 非圧縮形式（0x04 || X || Y）を渡す。**曲線上の点かどうかも検証される**ので、
	// 座標を直接詰めるより安全。
	buf := make([]byte, 1+2*p256CoordLen)
	buf[0] = 4
	copy(buf[1:], x)
	copy(buf[1+p256CoordLen:], y)

	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), buf)
	if err != nil {
		return nil, fmt.Errorf("公開鍵として解釈できない: %w", err)
	}

	return pub, nil
}

// --- context ---

type ctxKey struct{}

// WithUserID は userID を context に載せる。
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, userID)
}

// UserIDFrom は context から userID を取り出す。
func UserIDFrom(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ctxKey{}).(string)
	return v, ok
}

// --- ミドルウェア ---

// Middleware は Authorization ヘッダの Bearer トークンを検証する。
//
// publicPaths に完全一致するパスは検証しない（/health など、
// openapi.yaml で security: [] になっているもの）。
func Middleware(v *Verifier, publicPaths ...string) func(http.Handler) http.Handler {
	public := make(map[string]bool, len(publicPaths))
	for _, p := range publicPaths {
		public[p] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if public[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				unauthorized(w, "Authorization ヘッダが無いか Bearer ではない")
				return
			}

			claims, err := v.Verify(r.Context(), token)
			if err != nil {
				unauthorized(w, "トークンが受け入れられない")
				return
			}

			next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), claims.UserID)))
		})
	}
}

func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	// スキームは大文字小文字を区別しない（RFC 7235）
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}

	token := strings.TrimSpace(header[len(prefix):])

	return token, token != ""
}

func unauthorized(w http.ResponseWriter, detail string) {
	// RFC 6750: 401 には WWW-Authenticate を付ける
	w.Header().Set("WWW-Authenticate", `Bearer realm="physique-analytics"`)
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "about:blank",
		"title":  "認証が必要",
		"status": http.StatusUnauthorized,
		"detail": detail,
	})
}
