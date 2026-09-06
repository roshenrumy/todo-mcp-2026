package requeststate

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type State struct {
	Operation, TodoID, TargetListID string
	TodoVersion                     int
	IncompleteSubtaskIDs            []string
	ExpiresAt                       time.Time
}
type Codec struct{ secret []byte }

func New(secret string) Codec { return Codec{[]byte(secret)} }
func (c Codec) Encode(s State) (string, error) {
	b, e := json.Marshal(s)
	if e != nil {
		return "", e
	}
	p := base64.RawURLEncoding.EncodeToString(b)
	h := hmac.New(sha256.New, c.secret)
	h.Write([]byte(p))
	return p + "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil)), nil
}
func (c Codec) Decode(token string) (State, error) {
	var s State
	x := strings.Split(token, ".")
	if len(x) != 2 {
		return s, errors.New("invalid requestState")
	}
	h := hmac.New(sha256.New, c.secret)
	h.Write([]byte(x[0]))
	sig, e := base64.RawURLEncoding.DecodeString(x[1])
	if e != nil || !hmac.Equal(sig, h.Sum(nil)) {
		return s, errors.New("invalid requestState signature")
	}
	b, e := base64.RawURLEncoding.DecodeString(x[0])
	if e != nil {
		return s, errors.New("invalid requestState")
	}
	if e = json.Unmarshal(b, &s); e != nil {
		return s, e
	}
	if time.Now().After(s.ExpiresAt) {
		return s, errors.New("requestState expired")
	}
	return s, nil
}
