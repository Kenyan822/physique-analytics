package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/auth"
)

// 許可リストが実際の HTTP 経路で効くことを、サーバを立てて確かめる。
func TestE2E_許可リストがHTTPで効く(t *testing.T) {
	t.Parallel()

	v, key := newVerifierAllowing(t, "11111111-1111-1111-1111-111111111111")
	srv := httptest.NewServer(auth.Middleware(v)(okHandler()))
	defer srv.Close()

	call := func(sub string) int {
		c := validClaims()
		c["sub"] = sub
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/v1/exercises", nil)
		if err != nil {
			t.Fatalf("リクエストを作れない: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+signToken(t, key, c))
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("リクエスト失敗: %v", err)
		}
		defer func() { _ = res.Body.Close() }()

		return res.StatusCode
	}

	if got := call("11111111-1111-1111-1111-111111111111"); got != http.StatusOK {
		t.Errorf("許可した sub = %d, want 200", got)
	}
	if got := call(otherSub); got != http.StatusUnauthorized {
		t.Errorf("許可していない sub = %d, want 401", got)
	}
}
