package security

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizer_DangerousSchemes(t *testing.T) {
	sanitizer := NewSanitizer(DefaultConfig())

	dangerousURLs := []struct {
		url  string
		desc string
	}{
		{"javascript:alert('xss')", "javascript scheme"},
		{"JAVASCRIPT:alert('xss')", "javascript scheme uppercase"},
		{"JaVaScRiPt:alert('xss')", "javascript scheme mixed case"},
		{"data:text/html,<script>alert('xss')</script>", "data scheme"},
		{"DATA:text/html,<script>", "data scheme uppercase"},
		{"vbscript:msgbox('xss')", "vbscript scheme"},
		{"VBSCRIPT:msgbox('xss')", "vbscript scheme uppercase"},
		{"file:///etc/passwd", "file scheme"},
		{"FILE:///etc/passwd", "file scheme uppercase"},
	}

	for _, tc := range dangerousURLs {
		t.Run(tc.desc, func(t *testing.T) {
			err := sanitizer.Validate(tc.url)
			assert.ErrorIs(t, err, ErrDangerousScheme)
		})
	}
}

func TestSanitizer_PrivateIPs(t *testing.T) {
	t.Run("blocks private IPs by default", func(t *testing.T) {
		sanitizer := NewSanitizer(Config{
			MaxURLLength:    2048,
			AllowPrivateIPs: false,
		})

		privateURLs := []struct {
			url  string
			desc string
		}{
			{"http://localhost/path", "localhost"},
			{"http://LOCALHOST/path", "localhost uppercase"},
			{"http://127.0.0.1/path", "loopback IPv4"},
			{"http://127.0.0.255/path", "loopback range"},
			{"http://192.168.1.1/path", "private 192.168.x.x"},
			{"http://192.168.255.255/path", "private 192.168 max"},
			{"http://10.0.0.1/path", "private 10.x.x.x"},
			{"http://10.255.255.255/path", "private 10.x max"},
			{"http://172.16.0.1/path", "private 172.16.x.x"},
			{"http://172.31.255.255/path", "private 172.31.x.x"},
			{"http://[::1]/path", "loopback IPv6"},
			{"http://[0:0:0:0:0:0:0:1]/path", "loopback IPv6 expanded"},
			{"http://[fe80::1]/path", "link-local IPv6"},
		}

		for _, tc := range privateURLs {
			t.Run(tc.desc, func(t *testing.T) {
				err := sanitizer.Validate(tc.url)
				assert.ErrorIs(t, err, ErrPrivateIP)
			})
		}
	})

	t.Run("allows private IPs when configured", func(t *testing.T) {
		sanitizer := NewSanitizer(Config{
			MaxURLLength:    2048,
			AllowPrivateIPs: true,
		})

		err := sanitizer.Validate("http://localhost/path")
		assert.NoError(t, err)

		err = sanitizer.Validate("http://192.168.1.1/path")
		assert.NoError(t, err)

		err = sanitizer.Validate("http://10.0.0.1/path")
		assert.NoError(t, err)
	})

	t.Run("allows public IPs", func(t *testing.T) {
		sanitizer := NewSanitizer(Config{
			MaxURLLength:    2048,
			AllowPrivateIPs: false,
		})

		publicURLs := []string{
			"https://example.com/path",
			"https://203.0.113.1/path",
			"https://8.8.8.8/dns",
			"https://[2001:db8::1]/path",
		}

		for _, url := range publicURLs {
			t.Run(url, func(t *testing.T) {
				err := sanitizer.Validate(url)
				assert.NoError(t, err)
			})
		}
	})
}

func TestSanitizer_URLLength(t *testing.T) {
	t.Run("rejects URLs over max length", func(t *testing.T) {
		sanitizer := NewSanitizer(Config{
			MaxURLLength:    100,
			AllowPrivateIPs: true,
		})

		longURL := "https://example.com/" + strings.Repeat("a", 200)
		err := sanitizer.Validate(longURL)
		assert.ErrorIs(t, err, ErrURLTooLong)
	})

	t.Run("allows URLs under max length", func(t *testing.T) {
		sanitizer := NewSanitizer(Config{
			MaxURLLength:    100,
			AllowPrivateIPs: true,
		})

		shortURL := "https://example.com/short"
		err := sanitizer.Validate(shortURL)
		assert.NoError(t, err)
	})

	t.Run("uses default max length", func(t *testing.T) {
		sanitizer := NewSanitizer(DefaultConfig())

		// Default is 2048, so 2000 should be fine
		url := "https://example.com/" + strings.Repeat("a", 1980)
		err := sanitizer.Validate(url)
		assert.NoError(t, err)

		// 2100 should fail
		longURL := "https://example.com/" + strings.Repeat("a", 2100)
		err = sanitizer.Validate(longURL)
		assert.ErrorIs(t, err, ErrURLTooLong)
	})
}

func TestSanitizer_ValidURLs(t *testing.T) {
	sanitizer := NewSanitizer(DefaultConfig())

	validURLs := []string{
		"https://example.com",
		"https://example.com/path",
		"https://example.com/path?query=value",
		"https://example.com/path?query=value#fragment",
		"http://example.com:8080/path",
		"https://subdomain.example.com/path",
		"https://example.com/path/to/resource.html",
		"https://example.com/search?q=hello+world",
		"https://user:pass@example.com/path",
	}

	for _, url := range validURLs {
		t.Run(url, func(t *testing.T) {
			err := sanitizer.Validate(url)
			assert.NoError(t, err)
		})
	}
}

func TestSanitizer_InvalidURLs(t *testing.T) {
	sanitizer := NewSanitizer(DefaultConfig())

	invalidURLs := []struct {
		url  string
		desc string
	}{
		{"", "empty URL"},
		{"   ", "whitespace only"},
		{"not-a-url", "no scheme"},
		{"://example.com", "empty scheme"},
		{"ftp://example.com", "ftp scheme"},
		{"mailto:test@example.com", "mailto scheme"},
	}

	for _, tc := range invalidURLs {
		t.Run(tc.desc, func(t *testing.T) {
			err := sanitizer.Validate(tc.url)
			assert.Error(t, err)
		})
	}
}

func TestSanitizer_BlockedHosts(t *testing.T) {
	t.Run("blocks configured hosts", func(t *testing.T) {
		sanitizer := NewSanitizer(Config{
			MaxURLLength:    2048,
			AllowPrivateIPs: true,
			BlockedHosts:    []string{"evil.com", "malware.net"},
		})

		err := sanitizer.Validate("https://evil.com/path")
		assert.ErrorIs(t, err, ErrBlockedHost)

		err = sanitizer.Validate("https://malware.net/path")
		assert.ErrorIs(t, err, ErrBlockedHost)

		// Subdomain should also be blocked
		err = sanitizer.Validate("https://sub.evil.com/path")
		assert.ErrorIs(t, err, ErrBlockedHost)
	})

	t.Run("allows non-blocked hosts", func(t *testing.T) {
		sanitizer := NewSanitizer(Config{
			MaxURLLength:    2048,
			AllowPrivateIPs: true,
			BlockedHosts:    []string{"evil.com"},
		})

		err := sanitizer.Validate("https://good.com/path")
		assert.NoError(t, err)
	})
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, 2048, cfg.MaxURLLength)
	assert.False(t, cfg.AllowPrivateIPs)
	assert.Empty(t, cfg.BlockedHosts)
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.255", true},
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"172.15.0.1", false}, // Not in private range
		{"172.32.0.1", false}, // Not in private range
		{"192.168.0.1", true},
		{"192.168.255.255", true},
		{"8.8.8.8", false},
		{"203.0.113.1", false},
		{"::1", true},
		{"fe80::1", true},
		{"2001:db8::1", false},
	}

	for _, tc := range tests {
		t.Run(tc.ip, func(t *testing.T) {
			result := isPrivateIP(tc.ip)
			assert.Equal(t, tc.expected, result, "IP: %s", tc.ip)
		})
	}
}

func TestSanitizer_IntegrationWithModels(t *testing.T) {
	// Test that the sanitizer can be used as a drop-in validation
	sanitizer := NewSanitizer(DefaultConfig())

	t.Run("validates URL like models package", func(t *testing.T) {
		// Valid URL
		err := sanitizer.Validate("https://example.com/path")
		require.NoError(t, err)

		// Invalid scheme
		err = sanitizer.Validate("javascript:alert(1)")
		require.Error(t, err)

		// Private IP
		err = sanitizer.Validate("http://localhost/admin")
		require.Error(t, err)
	})
}
