package application

import (
	"context"
	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

// ScopedCatalog APIの部分一致結果から指定IDだけを返す一覧取得口。
type ScopedCatalog struct {
	Source interface {
		ListByID(context.Context, string, law.LawID) ([]law.Law, int, error)
	}
	ID law.LawID
}

func (c ScopedCatalog) ListAll(ctx context.Context, asof string) ([]law.Law, int, error) {
	return c.Source.ListByID(ctx, asof, c.ID)
}

// Selection CLIから渡された対象指定。両方空なら全件同期。
type Selection struct {
	LawID    string
	LawTitle string
}

func (s Selection) LawSelection() Selection { return s }
