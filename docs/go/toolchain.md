# Go のバージョンを上げるとき

## 上げる場所は `api/go.mod` の1行

```
toolchain go1.27.2
```

CI は `go-version-file: api/go.mod` を見ているので、ワークフロー側に
バージョンを書く必要はない。

## **golangci-lint も一緒に上げる**

Go を上げると Lint がこう落ちる。

```
could not load export data: internal error in importing "internal/goarch"
(cannot decode "internal/goarch", export data version 5 is greater than
 maximum supported version 4)
```

golangci-lint は**自分がビルドされた Go の型情報しか読めない**。
新しい Go の標準ライブラリ（`export data`）を読もうとして失敗する。

`.github/workflows/ci.yml` の

```yaml
- uses: golangci/golangci-lint-action@v9
  with:
    version: v2.14.0     # ← ここも上げる
```

**バージョンを `latest` にしない。** action が v1 系を落としてきて、
`.golangci.yaml` の `version: "2"` を読めずに落ちる。

## 手元の golangci-lint も上げる

CI だけ直しても、手元で `golangci-lint run ./...` を打つと同じエラーで落ちる。

```bash
golangci-lint --version
# golangci-lint has version 2.13.2 built with go1.27.1
```

`built with` が `api/go.mod` の toolchain より古いと落ちる。

```bash
brew upgrade golangci-lint
# または
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
```

**CI の `version:` と合わせること。** ずれていると「手元は通るのに CI が落ちる」
（またはその逆）になり、どちらを信じるか分からなくなる。

## 上げる理由はたいてい govulncheck

標準ライブラリの脆弱性は**自分のコードを一行も変えていなくても**出る。
`main` の CI が突然赤くなったらこれを疑う。

```bash
cd api && go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

`No vulnerabilities found.` が出れば通る。

## 順番

1. `api/go.mod` の toolchain を上げる
2. `go test ./... -race` と `govulncheck` をローカルで確認
3. Lint が落ちたら golangci-lint のバージョンを上げる
4. **toolchain の更新だけで PR を出す。** 機能変更を混ぜると、
   落ちたときにどちらが原因か分からなくなる
