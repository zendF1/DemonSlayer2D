package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"

	"demonslayer2d/internal/store"
)

// ______________________TEST LIMITER_____________________

func TestLimiterWindow(t *testing.T) {
	now := time.Unix(1000, 0)
	l := NewLimiter(2, time.Minute)
	l.now = func() time.Time { return now }

	for i := range 2 {
		if !l.Take("k") {
			t.Fatalf("take %d of 2 should succeed", i+1)
		}
	}
	if l.Take("k") {
		t.Fatal("should be blocked after 2 takes")
	}
	if !l.Take("other") {
		t.Fatal("keys must be independent")
	}
	now = now.Add(61 * time.Second)
	if !l.Take("k") {
		t.Fatal("should be allowed after the window passes")
	}
	now = now.Add(61 * time.Second)
	l.sweep()
	if n := len(l.hits); n != 0 {
		t.Fatalf("expired keys should be pruned, %d left", n)
	}
}

func TestLimiterRefund(t *testing.T) {
	l := NewLimiter(1, time.Minute)
	if !l.Take("k") {
		t.Fatal("first take should succeed")
	}
	l.Refund("k")
	if !l.Take("k") {
		t.Fatal("a refunded take must free its slot")
	}
	l.Refund("missing") // refunding an unknown key is a no-op
}

func TestLimiterTakeIsAtomic(t *testing.T) {
	l := NewLimiter(5, time.Minute)
	var ok atomic.Int32
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.Take("k") {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := ok.Load(); got != 5 {
		t.Fatalf("%d concurrent takes succeeded, want exactly 5", got)
	}
}

// ______________________TEST PASSWORD_____________________

var fastParams = Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}

func TestHashAndVerify(t *testing.T) {
	h, err := HashPassword("correct horse", fastParams)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=8192,t=1,p=1$") {
		t.Fatalf("unexpected encoding %q", h)
	}
	ok, err := VerifyPassword("correct horse", h)
	if err != nil || !ok {
		t.Fatalf("verify correct password: ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword("wrong horse", h)
	if err != nil || ok {
		t.Fatalf("verify wrong password: ok=%v err=%v", ok, err)
	}
}

func TestHashUsesRandomSalt(t *testing.T) {
	a, _ := HashPassword("same", fastParams)
	b, _ := HashPassword("same", fastParams)
	if a == b {
		t.Fatal("two hashes of the same password are identical")
	}
}

func TestVerifyMalformed(t *testing.T) {
	for _, h := range []string{"", "plain", "$argon2i$v=19$m=1,t=1,p=1$AAAA$AAAA", "$argon2id$v=19$m=x$AAAA$AAAA", "$argon2id$v=19$m=8,t=1,p=1$!!$AAAA"} {
		if _, err := VerifyPassword("x", h); !errors.Is(err, ErrBadHash) {
			t.Errorf("VerifyPassword(%q) err = %v, want ErrBadHash", h, err)
		}
	}
}

// ______________________TEST NAMES_____________________

func TestNormalizeUsername(t *testing.T) {
	ok := map[string]string{"Tanjiro": "tanjiro", "abcd": "abcd", "user_01": "user_01", strings.Repeat("a", 20): strings.Repeat("a", 20)}
	for in, want := range ok {
		got, err := NormalizeUsername(in)
		if err != nil || got != want {
			t.Errorf("NormalizeUsername(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"abc", strings.Repeat("a", 21), "has space", "dấu", "a-b-c", ""} {
		if _, err := NormalizeUsername(in); !errors.Is(err, ErrUsernameInvalid) {
			t.Errorf("NormalizeUsername(%q) err = %v, want ErrUsernameInvalid", in, err)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("12345678"); err != nil {
		t.Errorf("8 chars: %v", err)
	}
	if err := ValidatePassword(strings.Repeat("ư", 64)); err != nil {
		t.Errorf("64 runes: %v", err)
	}
	for _, p := range []string{"1234567", strings.Repeat("x", 65)} {
		if err := ValidatePassword(p); !errors.Is(err, ErrPasswordInvalid) {
			t.Errorf("ValidatePassword(len %d) err = %v", len(p), err)
		}
	}
}

func TestNormalizeCharName(t *testing.T) {
	d, k, err := NormalizeCharName("Thủy")
	if err != nil || d != "Thủy" || k != "thủy" {
		t.Fatalf("Thủy -> %q %q %v", d, k, err)
	}
	// Decomposed input (u + combining hook + ...) normalises to the same name.
	d2, k2, err := NormalizeCharName(norm.NFD.String("Thủy"))
	if err != nil || d2 != d || k2 != k {
		t.Fatalf("NFD input -> %q %q %v", d2, k2, err)
	}
	for _, ok := range []string{"Ab", "Tanjiro2", strings.Repeat("ê", 16)} {
		if _, _, err := NormalizeCharName(ok); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"A", strings.Repeat("a", 17), "có space", "x_y", "bad!", ""} {
		if _, _, err := NormalizeCharName(bad); !errors.Is(err, ErrNameInvalid) {
			t.Errorf("%q err = %v, want ErrNameInvalid", bad, err)
		}
	}
}

// ______________________TEST SERVICE_____________________

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
