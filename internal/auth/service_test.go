package auth

import (
	"context"
	"errors"
	"sync"
	"testing"

	"demonslayer2d/internal/store"
)

type fakeAccounts struct {
	mu     sync.Mutex
	byName map[string]store.Account
	nextID int64
}

func newFakeAccounts() *fakeAccounts { return &fakeAccounts{byName: map[string]store.Account{}} }

func (f *fakeAccounts) CreateAccount(_ context.Context, u, h string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byName[u]; ok {
		return 0, store.ErrUsernameTaken
	}
	f.nextID++
	f.byName[u] = store.Account{ID: f.nextID, Username: u, PasswordHash: h, Status: "active"}
	return f.nextID, nil
}

func (f *fakeAccounts) GetAccountByUsername(_ context.Context, u string) (store.Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.byName[u]
	if !ok {
		return store.Account{}, store.ErrNotFound
	}
	return a, nil
}

func (f *fakeAccounts) TouchLogin(context.Context, int64) error { return nil }

func TestRegisterAndLogin(t *testing.T) {
	ctx := context.Background()
	svc, err := NewService(newFakeAccounts(), fastParams, 5)
	if err != nil {
		t.Fatal(err)
	}
	id, err := svc.Register(ctx, "1.1.1.1", "Tanjiro", "kamado123")
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Login(ctx, "1.1.1.1", "TANJIRO", "kamado123")
	if err != nil || got != id {
		t.Fatalf("login: %d %v", got, err)
	}
	if _, err := svc.Register(ctx, "1.1.1.1", "tanjiro", "kamado123"); !errors.Is(err, store.ErrUsernameTaken) {
		t.Fatalf("duplicate register err = %v", err)
	}
	if _, err := svc.Register(ctx, "1.1.1.1", "ab", "kamado123"); !errors.Is(err, ErrUsernameInvalid) {
		t.Fatalf("bad username err = %v", err)
	}
	if _, err := svc.Register(ctx, "1.1.1.1", "nezuko", "short"); !errors.Is(err, ErrPasswordInvalid) {
		t.Fatalf("bad password err = %v", err)
	}
}

func TestLoginFailuresLookTheSame(t *testing.T) {
	ctx := context.Background()
	svc, _ := NewService(newFakeAccounts(), fastParams, 5)
	_, _ = svc.Register(ctx, "ip", "tanjiro", "kamado123")
	if _, err := svc.Login(ctx, "ip", "tanjiro", "wrongpass"); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("wrong password err = %v", err)
	}
	if _, err := svc.Login(ctx, "ip", "nobody", "whatever1"); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("unknown user err = %v", err)
	}
	if _, err := svc.Login(ctx, "ip", "x", "whatever1"); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("invalid username err = %v", err)
	}
}

func TestBannedAccountCannotLogin(t *testing.T) {
	ctx := context.Background()
	accs := newFakeAccounts()
	svc, _ := NewService(accs, fastParams, 5)
	_, _ = svc.Register(ctx, "ip", "banned", "kamado123")
	a := accs.byName["banned"]
	a.Status = "banned"
	accs.byName["banned"] = a
	if _, err := svc.Login(ctx, "ip", "banned", "kamado123"); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("banned login err = %v", err)
	}
}

func TestLoginRateLimit(t *testing.T) {
	ctx := context.Background()
	svc, _ := NewService(newFakeAccounts(), fastParams, 5)
	_, _ = svc.Register(ctx, "ip", "tanjiro", "kamado123")
	for i := 0; i < 5; i++ {
		_, _ = svc.Login(ctx, "ip", "tanjiro", "wrongpass")
	}
	if _, err := svc.Login(ctx, "ip", "tanjiro", "kamado123"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("6th attempt err = %v, want ErrRateLimited", err)
	}
	if _, err := svc.Login(ctx, "other-ip", "tanjiro", "kamado123"); err != nil {
		t.Fatalf("other IP should not be limited: %v", err)
	}
}

func TestRegisterRateLimit(t *testing.T) {
	ctx := context.Background()
	svc, _ := NewService(newFakeAccounts(), fastParams, 5)
	for i := 0; i < 5; i++ {
		if _, err := svc.Register(ctx, "ip", "user"+string(rune('a'+i))+"xx", "kamado123"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Register(ctx, "ip", "userfxx", "kamado123"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("6th registration err = %v", err)
	}
}

func TestConcurrentLoginAttemptsAreLimited(t *testing.T) {
	ctx := context.Background()
	svc, _ := NewService(newFakeAccounts(), fastParams, 5)
	if _, err := svc.Register(ctx, "ip", "tanjiro", "kamado123"); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	counts := map[error]int{}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Login(ctx, "ip", "tanjiro", "wrongpass")
			mu.Lock()
			counts[err]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if counts[ErrAuthFailed] != 5 || counts[ErrRateLimited] != 15 {
		t.Fatalf("20 parallel wrong passwords gave %v, want 5 ErrAuthFailed and 15 ErrRateLimited", counts)
	}
}

func TestSuccessfulLoginsDoNotUseTheLimit(t *testing.T) {
	ctx := context.Background()
	svc, _ := NewService(newFakeAccounts(), fastParams, 5)
	if _, err := svc.Register(ctx, "ip", "tanjiro", "kamado123"); err != nil {
		t.Fatal(err)
	}
	for i := range 10 {
		if _, err := svc.Login(ctx, "ip", "tanjiro", "kamado123"); err != nil {
			t.Fatalf("login %d: %v", i+1, err)
		}
	}
}
