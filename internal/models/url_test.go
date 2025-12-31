package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestURL_Validate(t *testing.T) {
	tests := []struct {
		name    string
		url     URL
		wantErr error
	}{
		{
			name: "valid url",
			url: URL{
				ShortCode:   "abc123",
				OriginalURL: "https://example.com/path",
			},
			wantErr: nil,
		},
		{
			name: "empty short code",
			url: URL{
				ShortCode:   "",
				OriginalURL: "https://example.com",
			},
			wantErr: ErrEmptyShortCode,
		},
		{
			name: "short code too long",
			url: URL{
				ShortCode:   "12345678901", // 11 chars
				OriginalURL: "https://example.com",
			},
			wantErr: ErrShortCodeLength,
		},
		{
			name: "empty original url",
			url: URL{
				ShortCode:   "abc123",
				OriginalURL: "",
			},
			wantErr: ErrEmptyURL,
		},
		{
			name: "invalid url format",
			url: URL{
				ShortCode:   "abc123",
				OriginalURL: "not-a-valid-url",
			},
			wantErr: ErrInvalidURL,
		},
		{
			name: "url without scheme",
			url: URL{
				ShortCode:   "abc123",
				OriginalURL: "example.com/path",
			},
			wantErr: ErrInvalidURL,
		},
		{
			name: "ftp scheme not allowed",
			url: URL{
				ShortCode:   "abc123",
				OriginalURL: "ftp://files.example.com",
			},
			wantErr: ErrInvalidURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.url.Validate()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestURL_IsExpired(t *testing.T) {
	now := time.Now()
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)

	tests := []struct {
		name      string
		expiresAt *time.Time
		expected  bool
	}{
		{
			name:      "no expiry",
			expiresAt: nil,
			expected:  false,
		},
		{
			name:      "expired",
			expiresAt: &past,
			expected:  true,
		},
		{
			name:      "not expired",
			expiresAt: &future,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := URL{
				ShortCode:   "test",
				OriginalURL: "https://example.com",
				ExpiresAt:   tt.expiresAt,
			}
			assert.Equal(t, tt.expected, u.IsExpired())
		})
	}
}

func TestURLCreate_Validate(t *testing.T) {
	tests := []struct {
		name    string
		create  URLCreate
		wantErr error
	}{
		{
			name: "valid with short code",
			create: URLCreate{
				OriginalURL: "https://example.com",
				ShortCode:   "custom",
			},
			wantErr: nil,
		},
		{
			name: "valid without short code",
			create: URLCreate{
				OriginalURL: "https://example.com",
			},
			wantErr: nil,
		},
		{
			name: "empty url",
			create: URLCreate{
				OriginalURL: "",
			},
			wantErr: ErrEmptyURL,
		},
		{
			name: "invalid url",
			create: URLCreate{
				OriginalURL: "not-valid",
			},
			wantErr: ErrInvalidURL,
		},
		{
			name: "short code too long",
			create: URLCreate{
				OriginalURL: "https://example.com",
				ShortCode:   "12345678901",
			},
			wantErr: ErrShortCodeLength,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.create.Validate()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsValidURL(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"https://example.com", true},
		{"http://example.com", true},
		{"https://example.com/path?query=1", true},
		{"https://sub.example.com:8080/path", true},
		{"ftp://files.example.com", false},
		{"example.com", false},
		{"not a url", false},
		{"", false},
		{"   ", false},
		{"javascript:alert(1)", false},
		{"file:///etc/passwd", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, tt.expected, isValidURL(tt.url))
		})
	}
}
