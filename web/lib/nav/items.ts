/** ナビの1項目。 */
export type NavItem = {
  href: string;
  /** 表示名。**略語や英語を避ける**（CSV / transfer のような内部語を出さない） */
  label: string;
  /** 補足。メニューを開いたときだけ出す */
  hint: string;
};

/**
 * 移動先の一覧。**並びは使う頻度の順**で、毎日触るものを先に置く。
 *
 * 記録（/log）はここに入れない。主要な操作としてボタンで常に見えているため、
 * メニューにも出すと同じものが2つ並ぶ。
 */
export const NAV_ITEMS: NavItem[] = [
  { href: "/", label: "今日", hint: "その日の記録をまとめて見る" },
  { href: "/meals", label: "食事", hint: "食べたものと残りの目標" },
  { href: "/body", label: "体組成", hint: "体重・体脂肪率・周囲長・疲労度" },
  { href: "/plan", label: "計画", hint: "3年計画のブロックと月ごとの目標" },
  { href: "/photos", label: "写真", hint: "身体写真を並べて見比べる" },
  { href: "/blood", label: "血液検査", hint: "検査結果と基準外の項目" },
  { href: "/settings", label: "設定", hint: "身長・フェーズ・栄養・大会など" },
  { href: "/transfer", label: "データ", hint: "CSV の取り込みと書き出し・設定の保存" },
];

/**
 * いまどの項目を開いているか。
 *
 * **前方一致で判定するが、トップだけは完全一致。** `/` を前方一致にすると
 * すべてのページで「today」が選択中になる。
 */
export function isCurrent(pathname: string, href: string): boolean {
  if (href === "/") return pathname === "/";

  return pathname === href || pathname.startsWith(`${href}/`);
}
