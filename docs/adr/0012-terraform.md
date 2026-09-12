# ADR-0012: インフラを Terraform で管理する

- Status: Accepted
- Date: 2026-09-12

## Context

このプロジェクトは6つのサービスにまたがる。

Google Cloud（Cloud Run / Artifact Registry / Secret Manager / Cloud Scheduler / IAM）、Supabase、Cloudflare R2、Vercel、Sentry、GitHub。

これらをコンソールから手動で設定すると、以下が起きる。

- **何をどう設定したか残らない。** 半年後に「なぜこの権限が付いているか」が分からなくなる
- **再現できない。** アカウントを作り直す、リージョンを変える、といった場面で同じ構成を復元できない
- **設定のドリフトに気づけない。** コンソールで一時的に変えた設定が本番に残り続ける

加えて、**Terraform の実務経験を得ることも目的に含む**（[ADR-0006](0006-go-backend.md) で Go を選んだのと同じ理由）。

## Decision

**インフラを Terraform で管理する。ただし管理対象を明確に線引きする。**

### 管理するもの

| プロバイダ | 対象 |
|---|---|
| `hashicorp/google` | Cloud Run サービス、Artifact Registry（クリーンアップポリシー含む）、Secret Manager の**シークレット定義のみ**、Cloud Scheduler、Workload Identity 連携、IAM |
| `cloudflare/cloudflare` | R2 バケット、CORS 設定 |
| `vercel/vercel` | プロジェクト設定（Root Directory / Production Branch）、環境変数 |
| `integrations/github` | ブランチ保護、リポジトリ設定、Actions Secrets の**定義のみ** |
| `jianyuan/sentry` | プロジェクト、アラートルール |
| `supabase/supabase` | プロジェクト設定（※下記の制約あり） |

### 管理しないもの

| 対象 | 理由 | 代わりに |
|---|---|---|
| **シークレットの値** | **state に平文で保存されるため**（下記） | 手動投入 |
| **DB スキーマ** | Terraform はテーブル定義の管理に向かない | `golang-migrate`（[05-インフラ設計.md](../05-インフラ設計.md) §7） |
| Supabase プロジェクト本体 | Free tier は2プロジェクトまでで作り直しが起きない。手動作成の方が単純 | コンソール |
| Apple Developer 関連 | **プロバイダが存在しない** | fastlane / 手動 |

### state の扱い

- **GCS バケットに置く**（既に GCP を使っており、無料枠内で収まる）
- バケットはバージョニングを有効にする（state 破損時の復旧用）

## シークレットを state に入れない理由

**Terraform の state はシークレットを平文で保存する。** これは仕様であり、暗号化バックエンドを使っても `terraform show` で読める。

したがって **Secret Manager には「箱」だけを Terraform で作り、値は手動またはCLIで投入する**。

```hcl
# 箱だけ作る（値は含めない）
resource "google_secret_manager_secret" "db_url" {
  secret_id = "supabase-pooler-url"
  replication { auto {} }
}
# google_secret_manager_secret_version は Terraform で管理しない
```

```bash
# 値は別途投入する
echo -n "$POOLER_URL" | gcloud secrets versions add supabase-pooler-url --data-file=-
```

同様に GitHub Actions Secrets、Vercel の機密環境変数も**箱だけ**を管理する。

## Alternatives considered

**A. 手動でコンソールから設定する**
却下。6サービスにまたがる構成を手動管理すると、設定の根拠が残らず再現もできない。**このプロジェクトは3年動かす前提**であり、初期の設定を半年後に思い出せる必要がある。

**B. Pulumi / CDK（プログラミング言語でインフラ定義）**
却下。Go で書けるため一貫性は高いが、**Terraform の方がプロバイダのエコシステムが厚く、実務での採用も多い**。学習目的としても Terraform が適している。

**C. Terraform を使うが全てを管理する（シークレットの値も含む）**
却下。state に平文で残るリスクが大きい。個人の体組成データを扱うプロジェクトで、認証情報の扱いを緩くしない。

## Consequences

**良い影響**

- **構成がコードとして残り、変更理由が git 履歴に残る**
- アカウント作り直し・リージョン変更に対応できる
- `terraform plan` で**意図しない差分（ドリフト）を検出できる**
- インフラ変更が PR レビューの対象になる

**悪い影響 / 受け入れるコスト**

- **初期セットアップのコストが高い。** コンソールで5分の作業に、Terraform では30分かかることがある
- **シークレットの投入が2段階になる**（箱を作る → 値を入れる）。運用手順が増える
- **Supabase プロバイダの機能は限定的。** テーブル定義・RLS・Auth の詳細設定は扱えないため、DB は別管理になる
- state ファイルの管理が必要（GCS バケット、バージョニング、ロック）

## Notes

`terraform apply` は**手動実行**とする。インフラ変更の頻度が低く、1人開発で state のロック競合が起きないため、CI から自動適用する必要がない。

ただし **`terraform plan` は CI で実行し、PR に差分を出す**。インフラ変更の影響を、マージ前にレビューできるようにするため。
