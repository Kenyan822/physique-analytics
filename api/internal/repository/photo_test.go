package repository_test

import (
	"testing"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

func photoInput(day int, pose openapi.PhotoPose) repository.PhotoInput {
	return repository.PhotoInput{
		Date:       jstDate(2035, 3, day),
		Pose:       pose,
		StorageKey: "photos/2035-03-01/" + string(pose) + ".jpg",
		MimeType:   "image/jpeg",
		ByteSize:   4_000_000,
	}
}

func TestCreatePhoto_登録して一覧で引ける(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewPhoto(testdb.Begin(t))

	for _, pose := range []openapi.PhotoPose{openapi.PoseFront, openapi.PoseSide, openapi.PoseBack} {
		if _, err := repo.Create(ctx, photoInput(1, pose)); err != nil {
			t.Fatalf("Create(%s): %v", pose, err)
		}
	}

	from, to := jstDate(2035, 3, 1), jstDate(2035, 3, 1)
	got, err := repo.List(ctx, &from, &to)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("件数 = %d, want 3", len(got))
	}
	// StorageKey は返すが、API には出さない（署名付きURLだけ返す）
	if got[0].StorageKey == "" {
		t.Error("StorageKey が空")
	}
}

func TestCreatePhoto_同じ日の同じ向きは弾く(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewPhoto(testdb.Begin(t))

	if _, err := repo.Create(ctx, photoInput(2, openapi.PoseFront)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := repo.Create(ctx, photoInput(2, openapi.PoseFront)); !repository.IsConflict(err) {
		t.Errorf("err = %v, want ErrConflict", err)
	}
	// **ここで別の向きを試さない。** 一意制約違反でトランザクションが
	// 中断されるため（SQLSTATE 25P02）、続きの文はすべて失敗する。
	// 本番は1リクエスト1トランザクションなので起きない
}

func TestCreatePhoto_別の向きなら入る(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewPhoto(testdb.Begin(t))

	for _, pose := range []openapi.PhotoPose{openapi.PoseFront, openapi.PoseSide} {
		if _, err := repo.Create(ctx, photoInput(3, pose)); err != nil {
			t.Fatalf("Create(%s): %v", pose, err)
		}
	}
}

func TestPhotoGuide_向きごとに直近の1枚(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewPhoto(testdb.Begin(t))

	// 3/1 と 3/10 に正面を撮る
	for _, day := range []int{1, 10} {
		in := photoInput(day, openapi.PoseFront)
		in.StorageKey = "photos/x" + string(rune('0'+day%10)) + ".jpg"
		if _, err := repo.Create(ctx, in); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	if _, err := repo.Create(ctx, photoInput(1, openapi.PoseSide)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Guide(ctx, jstDate(2035, 3, 20))
	if err != nil {
		t.Fatalf("Guide: %v", err)
	}

	byPose := map[openapi.PhotoPose]openapi.BodyPhoto{}
	for _, r := range got {
		byPose[r.Photo.Pose] = r.Photo
	}
	// 正面は直近（3/10）が返る
	if byPose[openapi.PoseFront].Date.Day() != 10 {
		t.Errorf("正面 = %v, want 3/10", byPose[openapi.PoseFront].Date)
	}
	if _, ok := byPose[openapi.PoseSide]; !ok {
		t.Error("側面が返っていない")
	}
}

func TestPhotoGuide_指定日以降は含めない(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewPhoto(testdb.Begin(t))

	if _, err := repo.Create(ctx, photoInput(15, openapi.PoseFront)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// **当日は含めない。** 撮り直すときに自分をガイドにしても意味が無い
	got, err := repo.Guide(ctx, jstDate(2035, 3, 15))
	if err != nil {
		t.Fatalf("Guide: %v", err)
	}
	for _, r := range got {
		if r.Photo.Date.Day() == 15 {
			t.Error("当日の写真がガイドに含まれている")
		}
	}
}

func TestDeletePhoto_論理削除する(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewPhoto(testdb.Begin(t))

	created, err := repo.Create(ctx, photoInput(20, openapi.PoseBack))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.SoftDelete(ctx, created.Photo.Id); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	if err := repo.SoftDelete(ctx, created.Photo.Id); !repository.IsNotFound(err) {
		t.Errorf("2回目: err = %v, want ErrNotFound", err)
	}

	// 消したら同じ日・同じ向きで撮り直せる
	if _, err := repo.Create(ctx, photoInput(20, openapi.PoseBack)); err != nil {
		t.Errorf("削除後の Create: %v", err)
	}
}
