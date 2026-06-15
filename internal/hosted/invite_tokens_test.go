package hosted

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateInviteTokenStoresOnlyHash(t *testing.T) {
	maxRuns := 3
	generated, err := GenerateInviteToken("reviewer", nil, &maxRuns, time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("generate invite token: %v", err)
	}
	if !strings.HasPrefix(generated.Token, InviteTokenPrefix) {
		t.Fatalf("token prefix = %q, want %q", generated.Token, InviteTokenPrefix)
	}
	id, secret, ok := ParseInviteToken(generated.Token)
	if !ok {
		t.Fatalf("parse generated token failed")
	}
	if id != generated.Invite.ID {
		t.Fatalf("parsed id = %q, want %q", id, generated.Invite.ID)
	}
	if generated.Invite.SecretHash == "" || generated.Invite.SecretHash == secret {
		t.Fatalf("secret hash was not stored separately from the secret")
	}
	if strings.Contains(generated.Invite.SecretHash, secret) {
		t.Fatalf("secret hash contains the plain secret")
	}
	if err := ValidateInviteToken(generated.Invite, secret, time.Now().UTC()); err != nil {
		t.Fatalf("validate generated token: %v", err)
	}
	if err := ValidateInviteToken(generated.Invite, "wrong-secret", time.Now().UTC()); err != ErrInviteUnavailable {
		t.Fatalf("wrong secret error = %v, want ErrInviteUnavailable", err)
	}
}
