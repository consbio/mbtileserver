package handlers

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// maxSignatureAge defines the maximum amount of time, in seconds
// that an HMAC signature can remain valid
const maxSignatureAge = time.Duration(15) * time.Minute

// hmacAuth wraps handler functions to provide request authentication. If
// -s/--secret-key is provided at startup, this function will enforce proper
// request signing. Otherwise, it will simply pass requests through to the
// handler.
func hmacAuth(hf handlerFunc, secretKey string, serviceId string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		// If secret key isn't set, allow all requests
		if secretKey == "" {
			return hf(w, r)
		}

		query := r.URL.Query()

		rawSignature := query.Get("signature")
		if rawSignature == "" {
			rawSignature = r.Header.Get("X-Signature")
		}
		if rawSignature == "" {
			return 400, errors.New("No signature provided")
		}

		rawSignDate := query.Get("date")
		if rawSignDate == "" {
			rawSignDate = r.Header.Get("X-Signature-Date")
		}
		if rawSignDate == "" {
			return 400, errors.New("No signature date provided")
		}

		signDate, err := time.Parse(time.RFC3339Nano, rawSignDate)
		if err != nil {
			return 400, errors.New("Signature date is not valid RFC3339")
		}
		if time.Now().Sub(signDate) > maxSignatureAge {
			return 400, errors.New("Signature is expired")
		}

		signatureParts := strings.SplitN(rawSignature, ":", 2)
		if len(signatureParts) != 2 {
			return 400, errors.New("Signature does not contain salt")
		}
		salt, signature := signatureParts[0], signatureParts[1]

		key := sha1.New()
		key.Write([]byte(salt + secretKey))
		hash := hmac.New(sha1.New, key.Sum(nil))
		message := fmt.Sprintf("%s:%s", rawSignDate, serviceId)
		hash.Write([]byte(message))
		checkSignature := base64.RawURLEncoding.EncodeToString(hash.Sum(nil))

		if subtle.ConstantTimeCompare([]byte(signature), []byte(checkSignature)) == 1 {
			return hf(w, r)
		}

		return 400, errors.New("Invalid signature")
	}
}
