package registry

import "time"

// RegisterRequest is the POST /admin/apps body.
type RegisterRequest struct {
	Name string `json:"name"`
}

// AppDTO names an application without its key.
type AppDTO struct {
	Name       string     `json:"name"`
	Active     bool       `json:"active"`
	Last4      string     `json:"last4"`
	CreatedAt  time.Time  `json:"created_at"`
	RotatedAt  *time.Time `json:"rotated_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	GraceUntil *time.Time `json:"grace_until,omitempty"`
}

// AppListDTO is the GET /admin/apps answer.
type AppListDTO struct {
	Apps []AppDTO `json:"apps"`
}

// CredentialsDTO carries an application's key. Returned once, on creation
// and on rotation, and never listed again.
type CredentialsDTO struct {
	Name       string     `json:"name"`
	Key        string     `json:"key"`
	GraceUntil *time.Time `json:"grace_until,omitempty"`
}

// ErrorDTO is the body of every refused request.
type ErrorDTO struct {
	Error string `json:"error"`
}
