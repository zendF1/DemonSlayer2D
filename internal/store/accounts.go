package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var (
	ErrNotFound      = errors.New("store: not found")
	ErrUsernameTaken = errors.New("store: username taken")
	ErrNameTaken     = errors.New("store: character name taken")
	ErrSlotOccupied  = errors.New("store: character slot occupied")
)

type Account struct {
	ID           int64
	Username     string
	PasswordHash string
	Status       string
}

func (s *Store) CreateAccount(ctx context.Context, username, passwordHash string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO accounts (username, password_hash) VALUES ($1, $2) RETURNING id`,
		username, passwordHash).Scan(&id)
	if c, ok := uniqueViolation(err); ok && c == "accounts_username_key" {
		return 0, ErrUsernameTaken
	}
	if err != nil {
		return 0, fmt.Errorf("store: create account: %w", err)
	}
	return id, nil
}

func (s *Store) GetAccountByUsername(ctx context.Context, username string) (Account, error) {
	var a Account
	err := s.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, status FROM accounts WHERE username = $1`,
		username).Scan(&a.ID, &a.Username, &a.PasswordHash, &a.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("store: get account: %w", err)
	}
	return a, nil
}

func (s *Store) TouchLogin(ctx context.Context, id int64) error {
	if _, err := s.pool.Exec(ctx, `UPDATE accounts SET last_login_at = now() WHERE id = $1`, id); err != nil {
		return fmt.Errorf("store: touch login: %w", err)
	}
	return nil
}
