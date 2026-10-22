package storage

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	otpVerificationTTL   = 10 * time.Minute
	otpForgotPasswordTTL = 10 * time.Minute

	prefixOTPVerification   = "otp:verification"
	prefixOTPForgotPassword = "otp:forgot_password"
)

type OTPData struct {
	OTP       string    `json:"otp"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (r *RedisClient) SaveEmailVerificationOTP(ctx context.Context, email, otp string) error {
	r.DeleteEmailVerificationOTP(ctx, email)

	data, err := json.Marshal(&OTPData{
		OTP:       otp,
		Email:     email,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(otpVerificationTTL),
	})
	if err != nil {
		return fmt.Errorf("failed to marshal verification OTP: %w", err)
	}

	key := fmt.Sprintf("%s:%s", prefixOTPVerification, email)
	if err := r.client.Set(ctx, key, data, otpVerificationTTL).Err(); err != nil {
		return fmt.Errorf("failed to save verification OTP: %w", err)
	}
	return nil
}

func (r *RedisClient) GetEmailVerificationOTP(ctx context.Context, email string) (*OTPData, error) {
	key := fmt.Sprintf("%s:%s", prefixOTPVerification, email)
	return r.getOTP(ctx, key)
}

func (r *RedisClient) VerifyEmailOTP(ctx context.Context, email, otp string) error {
	otpData, err := r.GetEmailVerificationOTP(ctx, email)
	if err != nil {
		return err
	}
	if otpData.OTP != otp {
		return fmt.Errorf("invalid OTP")
	}
	r.DeleteEmailVerificationOTP(ctx, email) // single-use
	return nil
}

func (r *RedisClient) DeleteEmailVerificationOTP(ctx context.Context, email string) error {
	key := fmt.Sprintf("%s:%s", prefixOTPVerification, email)
	return r.client.Del(ctx, key).Err()
}

func (r *RedisClient) SaveForgotPasswordOTP(ctx context.Context, email, otp string) error {
	r.DeleteForgotPasswordOTP(ctx, email)

	data, err := json.Marshal(&OTPData{
		OTP:       otp,
		Email:     email,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(otpForgotPasswordTTL),
	})
	if err != nil {
		return fmt.Errorf("failed to marshal forgot password OTP: %w", err)
	}

	key := fmt.Sprintf("%s:%s", prefixOTPForgotPassword, email)
	if err := r.client.Set(ctx, key, data, otpForgotPasswordTTL).Err(); err != nil {
		return fmt.Errorf("failed to save forgot password OTP: %w", err)
	}
	return nil
}

func (r *RedisClient) GetForgotPasswordOTP(ctx context.Context, email string) (*OTPData, error) {
	key := fmt.Sprintf("%s:%s", prefixOTPForgotPassword, email)
	return r.getOTP(ctx, key)
}

func (r *RedisClient) VerifyForgotPasswordOTP(ctx context.Context, email, otp string) error {
	otpData, err := r.GetForgotPasswordOTP(ctx, email)
	if err != nil {
		return err
	}
	if otpData.OTP != otp {
		return fmt.Errorf("invalid OTP")
	}
	r.DeleteForgotPasswordOTP(ctx, email)
	return nil
}

func (r *RedisClient) DeleteForgotPasswordOTP(ctx context.Context, email string) error {
	key := fmt.Sprintf("%s:%s", prefixOTPForgotPassword, email)
	return r.client.Del(ctx, key).Err()
}

func (r *RedisClient) ResendEmailVerificationOTP(ctx context.Context, email string) error {
	if _, err := r.GetEmailVerificationOTP(ctx, email); err != nil {
		return fmt.Errorf("no active verification session for this email")
	}
	return r.SaveEmailVerificationOTP(ctx, email, generateOTP())
}

func (r *RedisClient) ResendForgotPasswordOTP(ctx context.Context, email string) error {
	if _, err := r.GetForgotPasswordOTP(ctx, email); err != nil {
		return fmt.Errorf("no active password reset session for this email")
	}
	return r.SaveForgotPasswordOTP(ctx, email, generateOTP())
}

func generateOTP() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(900000))
	return fmt.Sprintf("%06d", n.Int64()+100000)
}

func (r *RedisClient) getOTP(ctx context.Context, key string) (*OTPData, error) {
	data, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("OTP not found or expired")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get OTP: %w", err)
	}
	var otpData OTPData
	if err := json.Unmarshal([]byte(data), &otpData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal OTP: %w", err)
	}
	return &otpData, nil
}
