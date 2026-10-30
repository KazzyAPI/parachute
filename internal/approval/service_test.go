package approval

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/state"
)

type memory struct{ document state.Document }

func (m *memory) Current() state.Document { return m.document }

func (m *memory) Update(change func(*state.Document) error) error { return change(&m.document) }

type listing []domain.Container

func (l listing) Containers(context.Context) ([]domain.Container, error) { return l, nil }

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Unix(0, 0) }

func TestReleaseByContainerAndByStack(t *testing.T) {
	keys := []state.FreezeKey{{Host: "local", Stack: "shop"}}
	web := domain.Container{ID: "1", Name: "shop-web-1", Labels: map[string]string{"com.docker.compose.project": "shop"}}
	build := func() (*Service, *memory) {
		store := &memory{document: state.Document{Freezes: []state.Freeze{{Transaction: "txn-1", Keys: keys}}}}
		return NewService(store, map[string]ContainerSource{"local": listing{web}}, fixedClock{}, slog.New(slog.NewTextHandler(io.Discard, nil))), store
	}
	service, store := build()
	code, body, err := service.Approve(context.Background(), ApproveRequest{Container: "shop-web-1"})
	if err != nil || code != http.StatusOK || body.(Released).Transactions[0] != "txn-1" || len(store.document.ActiveFreezes()) != 0 {
		t.Fatalf("container release: %d %v %v", code, body, err)
	}
	if code, _, _ := service.Approve(context.Background(), ApproveRequest{Container: "shop-web-1"}); code != http.StatusNotFound {
		t.Errorf("second release: %d", code)
	}
	service, store = build()
	if code, _, _ := service.Approve(context.Background(), ApproveRequest{Stack: "other"}); code != http.StatusNotFound {
		t.Errorf("unknown stack: %d", code)
	}
	if code, _, _ := service.Approve(context.Background(), ApproveRequest{Stack: "shop"}); code != http.StatusOK || len(store.document.ActiveFreezes()) != 0 {
		t.Errorf("stack release: %d", code)
	}
}
