package handlers

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// HandlerWrapper is a type definition for a function that takes an http.Handler
// and returns an http.Handler
type HandlerWrapper func(http.Handler) http.Handler

// maxSignatureAge defines the maximum amount of time, in seconds
// that an HMAC signature can remain valid
const maxSignatureAge = time.Duration(15) * time.Minute

// HMACAuthMiddleware wraps incoming requests to enforce HMAC signature authorization.
// All requests are expected to have either "signature" and "date" query parameters
// or "X-Signature" and "X-Signature-Date" headers.
func HMACAuthMiddleware(secretKey string, serviceSet *ServiceSet) HandlerWrapper {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			query := r.URL.Query()

			rawSignature := query.Get("signature")
			if rawSignature == "" {
				rawSignature = r.Header.Get("X-Signature")
			}
			if rawSignature == "" {
				http.Error(w, "No signature provided", http.StatusUnauthorized)
				return
			}

			rawSignDate := query.Get("date")
			if rawSignDate == "" {
				rawSignDate = r.Header.Get("X-Signature-Date")
			}
			if rawSignDate == "" {
				http.Error(w, "No signature date provided", http.StatusUnauthorized)
				return
			}

			signDate, err := time.Parse(time.RFC3339Nano, rawSignDate)
			if err != nil {
				http.Error(w, "Signature date is not valid RFC3339", http.StatusBadRequest)
				return
			}
			if time.Now().Sub(signDate) > maxSignatureAge {
				http.Error(w, "Signature is expired", http.StatusUnauthorized)
				return
			}

			signatureParts := strings.SplitN(rawSignature, ":", 2)
			if len(signatureParts) != 2 {
				http.Error(w, "Signature does not contain salt", http.StatusBadRequest)
				return
			}
			salt, signature := signatureParts[0], signatureParts[1]

			tilesetID := serviceSet.IDFromURLPath(r.URL.Path)

			key := sha1.New()
			key.Write([]byte(salt + secretKey))
			hash := hmac.New(sha1.New, key.Sum(nil))
			message := fmt.Sprintf("%s:%s", rawSignDate, tilesetID)
			hash.Write([]byte(message))
			checkSignature := base64.RawURLEncoding.EncodeToString(hash.Sum(nil))

			if subtle.ConstantTimeCompare([]byte(signature), []byte(checkSignature)) != 1 {
				// Signature is not valid for the requested resource
				// either tilesetID does not match in the signature, or date
				http.Error(w, "Signature not authorized for resource", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
