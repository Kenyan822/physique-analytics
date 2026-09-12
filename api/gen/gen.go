// Package gen は openapi.yaml からのコード生成を集約する。
//
// 生成コマンドはここに集約し、api/ 直下で go generate ./... を実行すれば
// すべての生成物が更新される状態を保つ。
package gen

//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../openapi.yaml
