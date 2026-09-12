package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// expectedAudience は Supabase がログイン済みユーザーに入れる aud。
const expectedAudience = "authenticated"

// parse はトークンを検証してクレームを取り出す。
func (v *Verifier) parse(ctx context.Context, token string) (Claims, error) {
	var claims jwt.RegisteredClaims

	_, err := jwt.ParseWithClaims(token, &claims,
		func(t *jwt.Token) (any, error) {
			kid, _ := t.Header["kid"].(string)
			if kid == "" {
				return nil, errors.New("kid が無い")
			}
			return v.keyFor(ctx, kid)
		},
		// **署名方式を ES256 に固定する。** 指定しないと、攻撃者が alg を
		// HS256 に差し替えて「公開鍵を共有シークレットとして」署名した
		// トークンを通せる（古典的な JWT の攻撃）。
		jwt.WithValidMethods([]string{jwt.SigningMethodES256.Alg()}),
		jwt.WithAudience(expectedAudience),
		// exp / nbf は既定で検証される。iat のずれは許容する
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return Claims{}, fmt.Errorf("トークンの検証に失敗: %w", err)
	}

	if claims.Subject == "" {
		return Claims{}, errors.New("sub が無い")
	}

	return Claims{UserID: claims.Subject}, nil
}
