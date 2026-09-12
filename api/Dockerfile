# syntax=docker/dockerfile:1

# --- build ---------------------------------------------------------------
# go.mod の toolchain ディレクティブでバージョンを固定しているので、
# ベースイメージのマイナーバージョンとずれても必要な toolchain を取りに行く。
FROM golang:1.27-alpine AS build

WORKDIR /src

# 依存だけを先にコピーすると、ソースだけ変えたときにこのレイヤーが再利用される
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# CGO を切って静的リンクにする。distroless/static に載せるために必須。
# -trimpath はビルドしたマシンのパスをバイナリから除く（再現性と情報漏れ対策）
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# --- runtime -------------------------------------------------------------
# distroless: シェルもパッケージマネージャも入っていない。
# 侵入されても実行できるものがほぼ無い状態にする（docs/06-技術選定.md）。
FROM gcr.io/distroless/static-debian12:nonroot

# TLS で Supabase / R2 に出ていくのに必要。distroless/static には同梱されている
COPY --from=build /out/server /server

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/server"]
