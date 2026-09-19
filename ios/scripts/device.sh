#!/bin/bash
# 繋がっている iPhone にビルドして入れ、起動する。
#
#   ./scripts/device.sh              ビルド → インストール → 起動
#   ./scripts/device.sh --no-launch  起動しない
#
# **デバイスは自動検出する。** 端末名や UDID をリポジトリに書かないため
# （公開リポジトリなので、個人の端末が特定できる情報を置かない）。
#
# 詳しい流れは docs/swift/dev-flow.md。
set -euo pipefail

BUNDLE_ID="io.github.kenyan822.physique"
DERIVED="${DERIVED_DATA:-/tmp/physique-dd-device}"
LAUNCH=1
[ "${1:-}" = "--no-launch" ] && LAUNCH=0

cd "$(dirname "$0")/.."

# ---- デバイスを見つける -------------------------------------------------

# devicectl の JSON は --json-output にファイルパスを要求する。
# /dev/stdout を渡すと壊れた JSON になるので一時ファイルを経由する
tmp="$(mktemp -t physique-devices)"
trap 'rm -f "$tmp"' EXIT

xcrun devicectl list devices --json-output "$tmp" >/dev/null 2>&1 || true

# 接続済みの iOS デバイスだけを拾う。tunnelState は抜き差しで変わるので
# 条件に入れない（切れていても xcodebuild が繋ぎ直す）。
#
# **mapfile を使わない。** macOS の /bin/bash は 3.2 で mapfile が無い
found=()
while IFS= read -r line; do
  [ -n "$line" ] && found+=("$line")
done < <(python3 "$(dirname "$0")/list-devices.py" "$tmp")

if [ ${#found[@]} -eq 0 ]; then
  cat >&2 <<'MSG'
iPhone が見つからない。

  1. USB で繋ぐ
  2. iPhone 側の「このコンピュータを信頼しますか？」で 信頼 → パスコード
  3. xcrun devicectl list devices で出るか確認する

初回の設定（デベロッパモード・証明書の信頼）は ios/README.md を参照。
MSG
  exit 1
fi

if [ ${#found[@]} -gt 1 ]; then
  echo "iPhone が複数繋がっている。1台だけにする。" >&2
  for f in "${found[@]}"; do echo "  - ${f%%	*}" >&2; done
  exit 1
fi

DEVICE="${found[0]%%	*}"
DEV_MODE="${found[0]##*	}"

if [ "$DEV_MODE" != "enabled" ]; then
  cat >&2 <<MSG
"$DEVICE" のデベロッパモードが無効（$DEV_MODE）。

  設定 > プライバシーとセキュリティ > デベロッパモード をオン（再起動する）

項目が出てこないときは、一度このスクリプトを実行してから設定を開き直す。
インストールを試みるまで項目が現れない。
MSG
  exit 1
fi

echo "==> $DEVICE"

# ---- ビルド -------------------------------------------------------------

# -allowProvisioningUpdates: 端末を Personal Team に登録して profile を作らせる。
# 付けないと初回に "no devices ... to generate a provisioning profile" で止まる
echo "==> ビルド"
xcodebuild -project Physique.xcodeproj -scheme Physique \
  -destination "platform=iOS,name=$DEVICE" \
  -derivedDataPath "$DERIVED" \
  -allowProvisioningUpdates build >/dev/null

APP="$DERIVED/Build/Products/Debug-iphoneos/Physique.app"
[ -d "$APP" ] || { echo "ビルド成果物が無い: $APP" >&2; exit 1; }

# ---- インストール -------------------------------------------------------

echo "==> インストール"
xcrun devicectl device install app --device "$DEVICE" "$APP" >/dev/null

if [ $LAUNCH -eq 0 ]; then
  echo "==> 完了（起動はしない）"
  exit 0
fi

# ---- 起動 ---------------------------------------------------------------

echo "==> 起動"
if ! out="$(xcrun devicectl device process launch --device "$DEVICE" "$BUNDLE_ID" 2>&1)"; then
  if printf '%s' "$out" | grep -q "not been explicitly trusted"; then
    cat >&2 <<'MSG'
開発者証明書が信頼されていない。iPhone で

  設定 > 一般 > VPNとデバイス管理 > デベロッパAPP
    > Apple Development: ... > 信頼

を済ませてから、もう一度実行する。アプリ自体はもう入っている。
MSG
    exit 1
  fi
  printf '%s\n' "$out" >&2
  exit 1
fi

echo "==> 完了"
