package access

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/chranama/MealCheck/internal/core"
	"github.com/chranama/MealCheck/internal/state"
)

const InviteTokenPrefix = "mc_inv_"

type GeneratedInviteToken struct {
	Invite core.InviteToken
	Token  string
}

func GenerateInviteToken(label string, expiresAt *time.Time, maxRuns *int, now time.Time) (GeneratedInviteToken, error) {
	id, err := randomTokenPart(8)
	if err != nil {
		return GeneratedInviteToken{}, err
	}
	secret, err := randomTokenPart(32)
	if err != nil {
		return GeneratedInviteToken{}, err
	}
	token := InviteTokenPrefix + id + "." + secret
	return GeneratedInviteToken{
		Invite: core.InviteToken{
			ID:         id,
			SecretHash: HashInviteSecret(secret),
			Label:      label,
			CreatedAt:  now.UTC(),
			ExpiresAt:  expiresAt,
			MaxRuns:    maxRuns,
		},
		Token: token,
	}, nil
}

func ParseInviteToken(value string) (id string, secret string, ok bool) {
	trimmed := strings.TrimSpace(value)
	rest, found := strings.CutPrefix(trimmed, InviteTokenPrefix)
	if !found {
		return "", "", false
	}
	id, secret, found = strings.Cut(rest, ".")
	if !found || id == "" || secret == "" {
		return "", "", false
	}
	return id, secret, true
}

func HashInviteSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return "sha256:" + base64.RawURLEncoding.EncodeToString(sum[:])
}

func InviteSecretMatches(invite core.InviteToken, secret string) bool {
	expected := HashInviteSecret(secret)
	return subtle.ConstantTimeCompare([]byte(invite.SecretHash), []byte(expected)) == 1
}

func ValidateInviteToken(invite core.InviteToken, secret string, now time.Time) error {
	if !InviteSecretMatches(invite, secret) {
		return state.ErrInviteUnavailable
	}
	if invite.RevokedAt != nil {
		return state.ErrInviteUnavailable
	}
	if invite.ExpiresAt != nil && !invite.ExpiresAt.After(now) {
		return state.ErrInviteUnavailable
	}
	if invite.MaxRuns != nil && invite.UsedRuns >= *invite.MaxRuns {
		return state.ErrInviteRunLimit
	}
	return nil
}

func randomTokenPart(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate invite token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
