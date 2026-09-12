#!/usr/bin/env bash
# GCP のセットアップ。docs/07-セットアップ.md §2 を実行可能にしたもの。
#
#   gcloud auth login --account <個人アカウント>
#   ./scripts/setup-gcp.sh
#
# 何度実行しても同じ結果になる（既に存在するものは作らない）。
set -euo pipefail

PROJECT_ID="${PROJECT_ID:-}"
REGION="${REGION:-asia-northeast1}"
REPO="${REPO:-Kenyan822/physique-analytics}"
BUDGET_JPY="${BUDGET_JPY:-1000}"

log()  { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
skip() { printf '    既に存在: %s\n' "$*"; }

# --- 事前確認 -------------------------------------------------------------
ACCOUNT=$(gcloud config get-value account 2>/dev/null)
log "実行アカウント: ${ACCOUNT}"
read -rp "    このアカウントで続行する？ [y/N] " ans
[[ "$ans" == "y" ]] || { echo "中止。gcloud config set account <ACCOUNT> で切り替えてから再実行"; exit 1; }

if [[ -z "$PROJECT_ID" ]]; then
  # プロジェクト ID はグローバルで一意。衝突しにくいよう乱数を足す
  PROJECT_ID="physique-analytics-$(LC_ALL=C tr -dc '0-9' </dev/urandom | head -c 6)"
fi

# --- 1. プロジェクト ------------------------------------------------------
log "プロジェクト: ${PROJECT_ID}"
if gcloud projects describe "$PROJECT_ID" >/dev/null 2>&1; then
  skip "$PROJECT_ID"
else
  gcloud projects create "$PROJECT_ID" --name="physique-analytics"
fi
gcloud config set project "$PROJECT_ID" >/dev/null
PROJECT_NUM=$(gcloud projects describe "$PROJECT_ID" --format='value(projectNumber)')

# --- 2. 課金 --------------------------------------------------------------
# 請求先アカウントには紐付けられるプロジェクト数の上限がある。
# 超えていると FAILED_PRECONDITION / "Cloud billing quota exceeded" で失敗する。
# 別の請求先を使うか、上限緩和を申請する:
#   https://support.google.com/code/contact/billing_quota_increase
log "課金アカウントの紐付け"
if [[ "$(gcloud billing projects describe "$PROJECT_ID" --format='value(billingEnabled)' 2>/dev/null)" == "True" ]]; then
  skip "課金は有効"
else
  gcloud billing accounts list
  read -rp "    紐付ける BILLING_ACCOUNT_ID: " BILLING_ACCOUNT_ID
  if ! gcloud billing projects link "$PROJECT_ID" --billing-account="$BILLING_ACCOUNT_ID"; then
    echo
    echo "  紐付けに失敗した。'Cloud billing quota exceeded' なら、この請求先が"
    echo "  紐付けられるプロジェクト数の上限に達している。別の請求先を選ぶか、"
    echo "  https://support.google.com/code/contact/billing_quota_increase で緩和を申請する。"
    exit 1
  fi
fi

# --- 3. API ---------------------------------------------------------------
log "API の有効化"
gcloud services enable \
  run.googleapis.com \
  artifactregistry.googleapis.com \
  secretmanager.googleapis.com \
  cloudscheduler.googleapis.com \
  iamcredentials.googleapis.com \
  billingbudgets.googleapis.com

# --- 4. Artifact Registry -------------------------------------------------
log "Artifact Registry"
if gcloud artifacts repositories describe physique --location="$REGION" >/dev/null 2>&1; then
  skip "physique"
else
  gcloud artifacts repositories create physique \
    --repository-format=docker \
    --location="$REGION" \
    --description="physique-analytics のコンテナイメージ"
fi

# --- 5. デプロイ用サービスアカウント ---------------------------------------
log "サービスアカウント"
SA="github-deployer@${PROJECT_ID}.iam.gserviceaccount.com"
if gcloud iam service-accounts describe "$SA" >/dev/null 2>&1; then
  skip "$SA"
else
  gcloud iam service-accounts create github-deployer --display-name="GitHub Actions deployer"
fi

# 権限は最小限に絞る。roles/editor のような広い権限は付けない
# 連続で叩くと etag が競合して一部が黙って失敗する。
# 失敗したら少し待って1回だけやり直す
for role in roles/run.admin roles/artifactregistry.writer roles/iam.serviceAccountUser; do
  if ! gcloud projects add-iam-policy-binding "$PROJECT_ID" \
       --member="serviceAccount:${SA}" --role="$role" --condition=None >/dev/null 2>&1; then
    sleep 3
    gcloud projects add-iam-policy-binding "$PROJECT_ID" \
      --member="serviceAccount:${SA}" --role="$role" --condition=None >/dev/null
  fi
  printf '    付与: %s\n' "$role"
done

# --- 6. Workload Identity -------------------------------------------------
# サービスアカウントキー（JSON）を発行しない。漏れたら失効させるまで悪用できるため。
# GitHub の OIDC トークンで都度短命の認証情報を得る（docs/05-インフラ設計.md §6）
log "Workload Identity"
if gcloud iam workload-identity-pools describe github --location=global >/dev/null 2>&1; then
  skip "pool: github"
else
  gcloud iam workload-identity-pools create github --location=global --display-name="GitHub Actions"
fi

if gcloud iam workload-identity-pools providers describe github \
     --location=global --workload-identity-pool=github >/dev/null 2>&1; then
  skip "provider: github"
else
  # attribute-condition が無いと他人のリポジトリからも認証できてしまう
  gcloud iam workload-identity-pools providers create-oidc github \
    --location=global \
    --workload-identity-pool=github \
    --display-name="GitHub" \
    --attribute-mapping="google.subject=assertion.sub,attribute.repository=assertion.repository" \
    --attribute-condition="assertion.repository=='${REPO}'" \
    --issuer-uri="https://token.actions.githubusercontent.com"
fi

gcloud iam service-accounts add-iam-policy-binding "$SA" \
  --role=roles/iam.workloadIdentityUser \
  --member="principalSet://iam.googleapis.com/projects/${PROJECT_NUM}/locations/global/workloadIdentityPools/github/attribute.repository/${REPO}" \
  >/dev/null

# --- 7. GitHub Secrets ----------------------------------------------------
log "GitHub Secrets"
WIF_PROVIDER="projects/${PROJECT_NUM}/locations/global/workloadIdentityPools/github/providers/github"
gh secret set WIF_PROVIDER        --repo "$REPO" --body "$WIF_PROVIDER"
gh secret set WIF_SERVICE_ACCOUNT --repo "$REPO" --body "$SA"
gh secret set GCP_PROJECT_ID      --repo "$REPO" --body "$PROJECT_ID"
gh secret set GCP_REGION          --repo "$REPO" --body "$REGION"

# --- 8. 予算アラート ------------------------------------------------------
# 個人プロジェクトで一番怖いのは設定ミスによる課金。無料枠内の想定でも必ず入れる
log "予算アラート（${BUDGET_JPY}円）"
BILLING_ACCOUNT=$(gcloud billing projects describe "$PROJECT_ID" --format='value(billingAccountName)' | sed 's|billingAccounts/||')
if gcloud billing budgets list --billing-account="$BILLING_ACCOUNT" --format='value(displayName)' 2>/dev/null | grep -q '^physique-analytics$'; then
  skip "予算: physique-analytics"
else
  gcloud billing budgets create \
    --billing-account="$BILLING_ACCOUNT" \
    --display-name="physique-analytics" \
    --budget-amount="${BUDGET_JPY}JPY" \
    --filter-projects="projects/${PROJECT_NUM}" \
    --threshold-rule=percent=0.5 \
    --threshold-rule=percent=0.9 \
    --threshold-rule=percent=1.0
fi

# --- 結果 -----------------------------------------------------------------
cat <<EOS

完了。

  PROJECT_ID           ${PROJECT_ID}
  PROJECT_NUMBER       ${PROJECT_NUM}
  REGION               ${REGION}
  SERVICE_ACCOUNT      ${SA}
  WIF_PROVIDER         ${WIF_PROVIDER}

GitHub Secrets に登録済み: WIF_PROVIDER / WIF_SERVICE_ACCOUNT / GCP_PROJECT_ID / GCP_REGION

次にやること:
  1. Supabase のプロジェクトを作る（docs/07-セットアップ.md §1）
  2. 接続文字列を Secret Manager に入れる:
       gcloud secrets create supabase-pooler-url --replication-policy=automatic
       printf '%s' '<接続文字列>' | gcloud secrets versions add supabase-pooler-url --data-file=-
EOS
