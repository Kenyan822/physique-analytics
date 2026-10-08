package repository_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

// **モックで PgError を作らず、実際に制約違反を起こす。** 本当に 23514 が返るかを確かめるため。
// 違反するとトランザクションが中断されるので、1テスト1違反にする
func TestPgError_実際の制約違反を判別する(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		sql        string
		args       []any
		check      bool
		fk         bool
		unique     bool
		constraint string
	}{
		{
			name:       "CHECK 違反",
			sql:        `insert into meals (date, source) values ('2032-05-01', 'guess')`,
			check:      true,
			constraint: "meals_source_check",
		},
		{
			name:       "外部キー違反",
			sql:        `insert into workout_sets (session_id, exercise_id, set_no, weight_kg, reps) values ($1, $2, 1, 50, 10)`,
			args:       []any{uuid.New(), uuid.New()},
			fk:         true,
			constraint: "workout_sets_session_id_fkey",
		},
		{
			name:       "一意制約違反",
			sql:        `insert into manual_targets (starts_on, protein_g, fat_g, carb_g) select starts_on, protein_g, fat_g, carb_g from (values ('1999-01-01'::date, 1, 1, 1)) v(starts_on, protein_g, fat_g, carb_g), generate_series(1, 2)`,
			unique:     true,
			constraint: "manual_targets_starts_on_key",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := testdb.Begin(t).Exec(t.Context(), tt.sql, tt.args...)
			if err == nil {
				t.Fatal("制約違反が起きていない")
			}

			// 呼び出し側は %w で包むので、包まれていても判別できること
			wrapped := fmt.Errorf("食事を記録できない: %w", err)

			if got := repository.IsCheckViolation(wrapped); got != tt.check {
				t.Errorf("IsCheckViolation = %v, want %v（err: %v）", got, tt.check, err)
			}
			if got := repository.IsForeignKeyViolation(wrapped); got != tt.fk {
				t.Errorf("IsForeignKeyViolation = %v, want %v", got, tt.fk)
			}
			if got := repository.IsUniqueViolation(wrapped); got != tt.unique {
				t.Errorf("IsUniqueViolation = %v, want %v", got, tt.unique)
			}
			if got := repository.ConstraintName(wrapped); got != tt.constraint {
				t.Errorf("ConstraintName = %q, want %q", got, tt.constraint)
			}
		})
	}
}

func TestPgError_制約違反でないエラーは何にも当たらない(t *testing.T) {
	t.Parallel()

	for _, err := range []error{nil, repository.ErrNotFound, repository.ErrConflict, fmt.Errorf("別のエラー")} {
		if repository.IsCheckViolation(err) || repository.IsForeignKeyViolation(err) ||
			repository.IsUniqueViolation(err) || repository.ConstraintName(err) != "" {
			t.Errorf("%v が制約違反として判別された", err)
		}
	}
}
