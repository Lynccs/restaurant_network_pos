package service

import (
	"errors"
	"log"
	"restaurant_network_pos/internal/models"
	"restaurant_network_pos/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

var svcLog = log.New(log.Writer(), "[AuthService] ", log.LstdFlags|log.Lshortfile)

var ErrInvalidCredentials = errors.New("invalid phone or pin")

type UserRepository interface {
	GetUserByPhone(phone string) (*repository.UserRow, error)
}

type AuthService struct {
	repo UserRepository
}

func NewAuthService(repo UserRepository) *AuthService {
	return &AuthService{repo: repo}
}

func (s *AuthService) Login(phone, pin string) (*models.User, error) {
	svcLog.Printf("Login: attempt for phone=%s", phone)

	row, err := s.repo.GetUserByPhone(phone)
	if err != nil {
		svcLog.Printf("Login: repo error for phone=%s: %v", phone, err)
		return nil, err
	}
	if row == nil {
		svcLog.Printf("Login: user not found for phone=%s", phone)
		return nil, ErrInvalidCredentials
	}

	if err = bcrypt.CompareHashAndPassword([]byte(row.PinHash), []byte(pin)); err != nil {
		svcLog.Printf("Login: invalid pin for phone=%s", phone)
		return nil, ErrInvalidCredentials
	}

	svcLog.Printf("Login: success for phone=%s id=%d role=%s", phone, row.User.ID, row.User.Role)
	return &row.User, nil
}
