package clock

import (
	"time"

	"github.com/okamyuji/egov-law-sync/internal/application"
	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

var _ application.Clock = System{}

// System application.Clockの実装。実時刻をJSTで返す
type System struct{}

// Now 実時刻をJSTで返す
func (System) Now() time.Time {
	return time.Now().In(law.JST)
}
