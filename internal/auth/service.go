package auth

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"demonslayer2d/internal/store"
)

var (
	ErrAuthFailed  = errors.New("auth: authentication failed")
	ErrRateLimited = errors.New("auth: rate limited")
)

type AccountStore interface {
	CreateAccount(ctx context.Context, username, passwordHash string) (int64, error)
	GetAccountByUsername(ctx context.Context, username string) (store.Account, error)
	TouchLogin(ctx context.Context, id int64) error
}

type Service struct {
	store      AccountStore
	params     Params
	sem        chan struct{} // bounds concurrent argon2 work to the CPU count
	dummyHash  string        // verified for unknown users so timing does not reveal existence
	loginFails *Limiter      // 5 failed logins per minute per ip|username
	registers  *Limiter      // registrations per hour per ip
}

func NewService(st AccountStore, p Params, registersPerHour int) (*Service, error) {
	dummy, err := HashPassword("dummy-password-for-timing", p)
	if err != nil {
		return nil, err
	}
	return &Service{
		store:      st,
		params:     p,
		sem:        make(chan struct{}, runtime.NumCPU()),
		dummyHash:  dummy,
		loginFails: NewLimiter(5, time.Minute),
		registers:  NewLimiter(registersPerHour, time.Hour),
	}, nil
}

func (s *Service) acquire(ctx context.Context) error {
	select {
	case s.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) release() { <-s.sem }

// Register creates an account and returns its id.
func (s *Service) Register(ctx context.Context, ip, username, password string) (int64, error) {
	u, err := NormalizeUsername(username)
	if err != nil {
		return 0, err
	}
	if err := ValidatePassword(password); err != nil {
		return 0, err
	}
	if !s.registers.Take(ip) {
		return 0, ErrRateLimited
	}
	if err := s.acquire(ctx); err != nil {
		return 0, err
	}
	hash, err := HashPassword(password, s.params)
	s.release()
	if err != nil {
		return 0, fmt.Errorf("auth: hash: %w", err)
	}
	return s.store.CreateAccount(ctx, u, hash)
}

// Login checks credentials and returns the account id.
// Unknown user, wrong password and banned account all return ErrAuthFailed.
func (s *Service) Login(ctx context.Context, ip, username, password string) (int64, error) {
	u, err := NormalizeUsername(username)
	if err != nil {
		return 0, ErrAuthFailed
	}
	// Reserve an attempt before doing any work so parallel logins cannot all pass
	// the check before a failure is recorded. Only a failed authentication keeps it.
	key := ip + "|" + u
	if !s.loginFails.Take(key) {
		return 0, ErrRateLimited
	}
	keep := false
	defer func() {
		if !keep {
			s.loginFails.Refund(key)
		}
	}()
	acc, err := s.store.GetAccountByUsername(ctx, u)
	notFound := errors.Is(err, store.ErrNotFound)
	if err != nil && !notFound {
		return 0, err
	}
	hash := acc.PasswordHash
	if notFound {
		hash = s.dummyHash
	}
	if err := s.acquire(ctx); err != nil {
		return 0, err
	}
	ok, err := VerifyPassword(password, hash)
	s.release()
	if err != nil {
		return 0, fmt.Errorf("auth: verify: %w", err)
	}
	if notFound || !ok || acc.Status != "active" {
		keep = true
		return 0, ErrAuthFailed
	}
	if err := s.store.TouchLogin(ctx, acc.ID); err != nil {
		return 0, err
	}
	return acc.ID, nil
}
