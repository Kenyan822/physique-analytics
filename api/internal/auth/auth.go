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
	"math/big"
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

	mu        sync.RWMutex
	keys      map[string]*ecdsa.PublicKey
	fetchedAt time.Time
}

// NewVerifier は Verifier を作る。
func NewVerifier(jwksURL string) *Verifier {
	return &Verifier{
		jwksURL: jwksURL,
		client:  &http.Client{Timeout: fetchTimeout},
		keys:    map[string]*ecdsa.PublicKey{},
	}
}

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

	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(x),
		Y:     new(big.Int).SetBytes(y),
	}, nil
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
