package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/simp-lee/ttsbridge/tts"
)

func TestHandleIndex_IncludesMutationAuthTokenSupport(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/", nil)
	rr := httptest.NewRecorder()

	handleIndex(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d, want=%d", rr.Code, http.StatusOK)
	}

	body := rr.Body.String()
	checks := []string{
		"id=\"webuiToken\"",
		"X-TTSBridge-Token",
		"authFetch('/api/install-ffmpeg'",
		"authFetch('/api/upload-music'",
		"authFetch('/api/synthesize'",
	}

	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Fatalf("index html missing %q", want)
		}
	}
}

func TestRequireMutationAuth(t *testing.T) {
	tests := []struct {
		name           string
		token          string
		remoteAddr     string
		host           string
		origin         string
		referer        string
		xToken         string
		authorization  string
		wantStatusCode int
	}{
		{
			name:           "token provided via X-TTSBridge-Token",
			token:          "secret-token",
			remoteAddr:     "203.0.113.10:12345",
			host:           "127.0.0.1:8080",
			xToken:         "secret-token",
			wantStatusCode: http.StatusNoContent,
		},
		{
			name:           "token provided via Authorization bearer",
			token:          "secret-token",
			remoteAddr:     "203.0.113.10:12345",
			host:           "127.0.0.1:8080",
			authorization:  "Bearer secret-token",
			wantStatusCode: http.StatusNoContent,
		},
		{
			name:           "token mismatch rejected",
			token:          "secret-token",
			remoteAddr:     "203.0.113.10:12345",
			host:           "127.0.0.1:8080",
			xToken:         "wrong-token",
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:           "token unset non-loopback rejected",
			token:          "",
			remoteAddr:     "203.0.113.10:12345",
			host:           "127.0.0.1:8080",
			origin:         "http://127.0.0.1:8080",
			wantStatusCode: http.StatusForbidden,
		},
		{
			name:           "token unset loopback same-origin allowed",
			token:          "",
			remoteAddr:     "127.0.0.1:12345",
			host:           "127.0.0.1:8080",
			origin:         "http://127.0.0.1:8080",
			wantStatusCode: http.StatusNoContent,
		},
		{
			name:           "token unset loopback cross-origin rejected",
			token:          "",
			remoteAddr:     "127.0.0.1:12345",
			host:           "127.0.0.1:8080",
			origin:         "https://evil.example",
			wantStatusCode: http.StatusForbidden,
		},
		{
			name:           "token unset loopback without origin or referer rejected",
			token:          "",
			remoteAddr:     "127.0.0.1:12345",
			host:           "127.0.0.1:8080",
			wantStatusCode: http.StatusForbidden,
		},
		{
			name:           "loopback hostname without port with same-site referer allowed",
			token:          "",
			remoteAddr:     "localhost",
			host:           "localhost:8080",
			referer:        "http://localhost:8080/index.html",
			wantStatusCode: http.StatusNoContent,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TTSBRIDGE_WEBUI_TOKEN", tc.token)

			req := httptest.NewRequest(http.MethodPost, "http://"+tc.host+"/api/synthesize", nil)
			req.RemoteAddr = tc.remoteAddr
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.referer != "" {
				req.Header.Set("Referer", tc.referer)
			}
			if tc.xToken != "" {
				req.Header.Set("X-TTSBridge-Token", tc.xToken)
			}
			if tc.authorization != "" {
				req.Header.Set("Authorization", tc.authorization)
			}

			rr := httptest.NewRecorder()
			handler := requireMutationAuth(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})
			handler(rr, req)

			if rr.Code != tc.wantStatusCode {
				t.Fatalf("status=%d, want=%d", rr.Code, tc.wantStatusCode)
			}
		})
	}
}

func TestIsLoopbackRequest(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		want       bool
	}{
		{name: "ipv4 loopback", remoteAddr: "127.0.0.1:8080", want: true},
		{name: "ipv6 loopback", remoteAddr: "[::1]:8080", want: true},
		{name: "localhost host", remoteAddr: "localhost", want: true},
		{name: "remote host", remoteAddr: "203.0.113.2:8080", want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/api/synthesize", nil)
			req.RemoteAddr = tc.remoteAddr
			if got := isLoopbackRequest(req); got != tc.want {
				t.Fatalf("isLoopbackRequest()=%v, want=%v", got, tc.want)
			}
		})
	}
}

func TestPreloadProviderVoices_TimeoutDoesNotBlockStartup(t *testing.T) {
	originalTimeout := startupVoicePreloadTimeout
	startupVoicePreloadTimeout = 80 * time.Millisecond
	t.Cleanup(func() {
		startupVoicePreloadTimeout = originalTimeout
	})

	cachedVoices = make(map[string][]tts.Voice)

	started := time.Now()
	preloadProviderVoices("slow-provider", func(ctx context.Context, _ string) ([]tts.Voice, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	elapsed := time.Since(started)

	if _, ok := cachedVoices["slow-provider"]; ok {
		t.Fatalf("超时预加载不应写入缓存")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("预加载应在超时后快速返回，实际耗时: %v", elapsed)
	}
}

func TestPreloadProviderVoices_SuccessCachesVoices(t *testing.T) {
	cachedVoices = make(map[string][]tts.Voice)
	want := []tts.Voice{{Name: "demo"}}

	preloadProviderVoices("test-provider", func(_ context.Context, _ string) ([]tts.Voice, error) {
		return want, nil
	})

	got, ok := cachedVoices["test-provider"]
	if !ok {
		t.Fatalf("成功预加载后应写入缓存")
	}
	if len(got) != len(want) {
		t.Fatalf("缓存语音数量不正确: got=%d want=%d", len(got), len(want))
	}
}
