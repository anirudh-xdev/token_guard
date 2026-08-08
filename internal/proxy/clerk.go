package proxy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/jwt"
	"github.com/clerk/clerk-sdk-go/v2/user"
)

type clerkIdentity struct {
	Subject string
	Email   string
	Name    string
}

const (
	clerkVerifyTimeout = 8 * time.Second
	clerkVerifyLeeway  = 10 * time.Second
)

func (h *Handler) verifyClerkBearer(parent context.Context, authorization string) (clerkIdentity, error) {
	if h == nil || strings.TrimSpace(h.clerkSecretKey) == "" {
		return clerkIdentity{}, errors.New("clerk is not configured")
	}
	token := strings.TrimSpace(authorization)
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	if token == "" {
		return clerkIdentity{}, errors.New("missing bearer token")
	}

	// Detach from the inbound request cancel. React remounts / aborted fetches
	// otherwise cancel JWKS lookup and surface as "Invalid Clerk session".
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), clerkVerifyTimeout)
	defer cancel()

	claims, err := jwt.Verify(ctx, &jwt.VerifyParams{
		Token:  token,
		Leeway: clerkVerifyLeeway,
	})
	if err != nil {
		return clerkIdentity{}, fmt.Errorf("verify clerk token: %w", err)
	}
	if claims == nil || strings.TrimSpace(claims.Subject) == "" {
		return clerkIdentity{}, errors.New("clerk token missing subject")
	}

	identity := clerkIdentity{Subject: claims.Subject}
	usr, err := user.Get(ctx, claims.Subject)
	if err != nil {
		// Token is valid; profile fetch is best-effort for email/name.
		return identity, nil
	}
	if usr != nil {
		if usr.FirstName != nil || usr.LastName != nil {
			parts := make([]string, 0, 2)
			if usr.FirstName != nil && strings.TrimSpace(*usr.FirstName) != "" {
				parts = append(parts, strings.TrimSpace(*usr.FirstName))
			}
			if usr.LastName != nil && strings.TrimSpace(*usr.LastName) != "" {
				parts = append(parts, strings.TrimSpace(*usr.LastName))
			}
			identity.Name = strings.Join(parts, " ")
		}
		if identity.Name == "" && usr.Username != nil {
			identity.Name = strings.TrimSpace(*usr.Username)
		}
		for _, addr := range usr.EmailAddresses {
			if addr == nil || strings.TrimSpace(addr.EmailAddress) == "" {
				continue
			}
			if usr.PrimaryEmailAddressID != nil && addr.ID == *usr.PrimaryEmailAddressID {
				identity.Email = strings.TrimSpace(strings.ToLower(addr.EmailAddress))
				break
			}
			if identity.Email == "" {
				identity.Email = strings.TrimSpace(strings.ToLower(addr.EmailAddress))
			}
		}
	}
	return identity, nil
}

func initClerk(secretKey string) {
	secretKey = strings.TrimSpace(secretKey)
	if secretKey == "" {
		return
	}
	clerk.SetKey(secretKey)
	log.Printf("clerk JWT verification configured")
}
