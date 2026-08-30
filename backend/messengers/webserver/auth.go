package webserver

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
)

const bearerTokenHashPrefix = "sha256:"

// bearerTokenHash is the decoded SHA-256 hash of one configured bearer token.
type bearerTokenHash [sha256.Size]byte

// authenticator authenticates an HTTP request using one configured method.
type authenticator interface {
	// authenticate reports whether the request satisfies this authenticator.
	authenticate(*http.Request) bool

	// challenge returns the value advertised in WWW-Authenticate when no
	// configured credential accepts a request.
	challenge() string
}

// authCredential associates a logging ID with one authentication method.
type authCredential struct {
	id            string
	authenticator authenticator
}

// bearerTokenAuthenticator authenticates requests against one decoded
// bearer-token hash.
type bearerTokenAuthenticator struct {
	hash bearerTokenHash
}

type bearerClientIDContextKey struct{}

// parseAuthCredentials validates and compiles configured authentication
// methods before the webserver starts accepting connections.
func parseAuthCredentials(config []ConfigAuth) ([]authCredential, error) {
	if len(config) == 0 {
		return nil, nil
	}

	ids := make(map[string]struct{}, len(config))
	credentials := make([]authCredential, 0, len(config))
	for i, auth := range config {
		if auth.ID == "" {
			return nil, fmt.Errorf("auth[%d].id is required", i)
		}
		if _, exists := ids[auth.ID]; exists {
			return nil, fmt.Errorf("auth[%d].id %q is duplicated", i, auth.ID)
		}
		ids[auth.ID] = struct{}{}

		numMethods := 0
		var configuredAuthenticator authenticator
		if auth.BearerTokenHash != "" {
			numMethods++

			if !strings.HasPrefix(auth.BearerTokenHash, bearerTokenHashPrefix) {
				return nil, fmt.Errorf("auth[%d].bearerTokenHash must start with %q", i, bearerTokenHashPrefix)
			}
			decoded, err := hex.DecodeString(strings.TrimPrefix(auth.BearerTokenHash, bearerTokenHashPrefix))
			if err != nil || len(decoded) != sha256.Size {
				return nil, fmt.Errorf("auth[%d].bearerTokenHash must contain a 32-byte hexadecimal SHA-256 hash", i)
			}
			var hash bearerTokenHash
			copy(hash[:], decoded)
			configuredAuthenticator = bearerTokenAuthenticator{hash: hash}
		}
		if numMethods != 1 {
			return nil, fmt.Errorf("auth[%d] contains %d authentication methods; exactly one is required", i, numMethods)
		}

		credentials = append(credentials, authCredential{
			id:            auth.ID,
			authenticator: configuredAuthenticator,
		})
	}
	return credentials, nil
}

// authMiddleware rejects API requests that do not satisfy any configured
// authentication method.
func (s *Webserver) authMiddleware(inner http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientID, authenticated := s.authenticate(r)
		if !authenticated {
			s.addAuthChallenges(w.Header())
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), bearerClientIDContextKey{}, clientID)
		inner.ServeHTTP(w, r.WithContext(ctx))
	})
}

// addAuthChallenges advertises each configured authentication method once.
func (s *Webserver) addAuthChallenges(header http.Header) {
	added := make(map[string]struct{}, len(s.authCredentials))
	for _, credential := range s.authCredentials {
		challenge := credential.authenticator.challenge()
		if _, exists := added[challenge]; exists {
			continue
		}
		header.Add("WWW-Authenticate", challenge)
		added[challenge] = struct{}{}
	}
}

// authenticate tries every configured credential and returns the matching ID.
func (s *Webserver) authenticate(r *http.Request) (string, bool) {
	authenticated := false
	clientID := ""
	for _, credential := range s.authCredentials {
		if credential.authenticator.authenticate(r) {
			authenticated = true
			clientID = credential.id
		}
	}
	return clientID, authenticated
}

// authenticate verifies the request's bearer token against this method's
// configured hash.
func (a bearerTokenAuthenticator) authenticate(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.ContainsAny(token, " \t\r\n") {
		return false
	}
	presentedHash := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(presentedHash[:], a.hash[:]) == 1
}

// challenge returns the HTTP authentication challenge for bearer tokens.
func (bearerTokenAuthenticator) challenge() string {
	return "Bearer"
}

// bearerClientID returns the authenticated client ID attached by the bearer
// authentication middleware.
func bearerClientID(ctx context.Context) string {
	clientID, _ := ctx.Value(bearerClientIDContextKey{}).(string)
	return clientID
}
