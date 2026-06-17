package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// User is an application account.
type User struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	Role         string     `json:"role"`
	CreatedAt    time.Time  `json:"created_at"`
	LastLoginAt  *time.Time `json:"last_login_at"`
}

// ErrUserNotFound is returned when a lookup finds no matching user.
var ErrUserNotFound = errors.New("user not found")

// CreateUser inserts a user, or updates the password/role if the username
// already exists (upsert), returning the id.
func (s *Store) CreateUser(ctx context.Context, username, passwordHash, role string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO app_user (username, password_hash, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (username) DO UPDATE
		  SET password_hash = EXCLUDED.password_hash, role = EXCLUDED.role
		RETURNING id`,
		username, passwordHash, role,
	).Scan(&id)
	return id, err
}

// GetUserByUsername loads a user by name.
func (s *Store) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id, username, password_hash, role, created_at, last_login_at
		FROM app_user WHERE username = $1`, username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// TouchLogin records the last login time.
func (s *Store) TouchLogin(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE app_user SET last_login_at = now() WHERE id = $1`, id)
	return err
}

// CountUsers returns the number of application users.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM app_user`).Scan(&n)
	return n, err
}
