package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"html"
	"io"
	"log/slog"
	"net/http"
	"regexp"
)

// MaxRequestBodySize is the maximum allowed request body size (1MB).
const MaxRequestBodySize = 1 << 20 // 1MB

// SecurityHeaders adds standard security headers to all responses.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "0") // Disabled per modern best practice
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		next.ServeHTTP(w, r)
	})
}

// LimitBody wraps the request body with a size limit for state-changing methods.
func LimitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodySize)
		}
		next.ServeHTTP(w, r)
	})
}

// noListingFS wraps http.Dir to prevent directory listings.
type noListingFS struct {
	fs http.FileSystem
}

// Open opens the named file, returning 404 for directories without index.html.
func (nfs noListingFS) Open(name string) (http.File, error) {
	f, err := nfs.fs.Open(name)
	if err != nil {
		return nil, err
	}

	stat, err := f.Stat()
	if err != nil {
		if closeErr := f.Close(); closeErr != nil {
			slog.Warn("security: failed to close file after stat error", "name", name, "error", closeErr)
		}
		return nil, err
	}

	if stat.IsDir() {
		// Check for index.html; if missing, deny listing
		index, err := nfs.fs.Open(name + "/index.html")
		if err != nil {
			if closeErr := f.Close(); closeErr != nil {
				slog.Warn("security: failed to close directory file", "name", name, "error", closeErr)
			}
			return nil, err
		}
		if err := index.Close(); err != nil {
			slog.Warn("security: failed to close index file", "name", name, "error", err)
		}
	}

	return f, nil
}

// NoListingFileServer returns an http.FileServer that does not list directories.
func NoListingFileServer(root string) http.Handler {
	return http.FileServer(noListingFS{http.Dir(root)})
}

// EscapeHTML escapes a string for safe embedding in HTML.
func EscapeHTML(s string) string {
	return html.EscapeString(s)
}

// ValidName checks that a name is a valid Kubernetes resource name
// (lowercase alphanumeric + hyphens, max 63 chars, not starting/ending with hyphen).
var validNameRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

func ValidName(name string) bool {
	return validNameRegex.MatchString(name)
}

// GenerateCSRFToken generates a random hex token for CSRF protection.
func GenerateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
