package clock

import (
	"testing"
	"time"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

func TestSystemNowIsJST(t *testing.T) {
	now := System{}.Now()
	if now.Location().String() != law.JST.String() {
		t.Fatalf("location=%v want=%v", now.Location(), law.JST)
	}
	if diff := time.Since(now); diff < 0 || diff > time.Minute {
		t.Fatalf("now=%v not close to current time", now)
	}
}
