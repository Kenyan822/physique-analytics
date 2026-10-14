package timeutil_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// truncateDay は「今日」を UTC 境界で切ってしまう書き方。
var truncateDay = regexp.MustCompile(`Truncate\(24\s*\*\s*time\.Hour\)`)

// TestTruncateは使わない はソースを走査して再発を止める。
//
// **同じバグを5箇所で踏んだ**（#311）。`Truncate` は絶対時刻を切るので
// ロケーションを見ず、JST では9時に丸まる。朝9時前に呼ぶと前日になる。
//
// golangci-lint の forbidigo は**関数名しか見ず引数を見ない**ので、
// この形は拾えない。型でも防げない（`time.Time` のまま正しく見える）。
// 残る手段がソースの走査しかない。
func TestTruncateは使わない(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	var found []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// 生成物は直す先が違う
			if d.Name() == "gen" || d.Name() == ".git" {
				return fs.SkipDir
			}

			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// この検査そのものと、使ってはいけない理由を書いた場所は対象外
		if strings.HasSuffix(path, "guard_test.go") || strings.HasSuffix(path, "today.go") {
			return nil
		}

		b, err := os.ReadFile(path) //nolint:gosec // テスト内でソースを読むだけ
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			// コメントで言及するのは許す
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if truncateDay.MatchString(line) {
				found = append(found, filepath.ToSlash(path)+":"+itoa(i+1))
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("走査できない: %v", err)
	}

	if len(found) > 0 {
		t.Errorf("「今日」は timeutil.Today() / StartOfDay() で出す。"+
			"Truncate は JST で9時に丸まる（ADR-0013 / #311）:\n  %s",
			strings.Join(found, "\n  "))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}

	return string(b)
}
