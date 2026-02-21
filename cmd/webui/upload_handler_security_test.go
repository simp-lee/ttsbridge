package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simp-lee/ttsbridge/tts"
)

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func makeUploadRequest(t *testing.T, filename string) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	headers := textproto.MIMEHeader{}
	headers.Set("Content-Disposition", fmt.Sprintf(`form-data; name="music"; filename="%s"`, filename))
	headers.Set("Content-Type", "application/octet-stream")
	part, err := writer.CreatePart(headers)
	if err != nil {
		t.Fatalf("CreatePart failed: %v", err)
	}
	if _, err := part.Write([]byte("dummy audio")); err != nil {
		t.Fatalf("write multipart data failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/upload-music", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func parseError(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var payload map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	return payload["error"]
}

func TestHandleUploadMusicRejectsAbsolutePathFilename(t *testing.T) {
	req := makeUploadRequest(t, "/tmp/evil.mp3")
	rr := httptest.NewRecorder()

	handleUploadMusic(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if msg := parseError(t, rr); msg != "非法文件名" {
		t.Fatalf("unexpected error message: %s", msg)
	}
}

func TestHandleUploadMusicRejectsFilenameWithSeparator(t *testing.T) {
	req := makeUploadRequest(t, "..\\evil.mp3")
	rr := httptest.NewRecorder()

	handleUploadMusic(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if msg := parseError(t, rr); msg != "非法文件名" {
		t.Fatalf("unexpected error message: %s", msg)
	}
}

func TestHandleUploadMusicCleansMultipartFormAfterParseSuccess(t *testing.T) {
	originalCleanup := cleanupMultipartForm
	defer func() { cleanupMultipartForm = originalCleanup }()

	called := false
	cleanupMultipartForm = func(r *http.Request) {
		called = true
		if r == nil || r.MultipartForm == nil {
			t.Fatalf("expected multipart form to be present")
		}
	}

	req := makeUploadRequest(t, "music.txt")
	rr := httptest.NewRecorder()

	handleUploadMusic(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !called {
		t.Fatalf("expected multipart cleanup to be called")
	}
}

func TestHandleSynthesizeRejectsBackgroundMusicPathOutsideTempDir(t *testing.T) {
	tempFile, err := os.CreateTemp("", "outside_music_*.mp3")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	payload := map[string]interface{}{
		"text":     "hello",
		"voice":    "en-US-AvaMultilingualNeural",
		"provider": "edgetts",
		"background_music": map[string]interface{}{
			"music_path": tempFile.Name(),
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/synthesize", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handleSynthesize(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if msg := parseError(t, rr); !strings.Contains(msg, "背景音乐文件路径非法") {
		t.Fatalf("unexpected error message: %s", msg)
	}
}

func TestHandleUploadMusicRejectsOversizedRequestBody(t *testing.T) {
	originalCleanup := cleanupMultipartForm
	defer func() { cleanupMultipartForm = originalCleanup }()

	called := false
	cleanupMultipartForm = func(r *http.Request) {
		called = true
	}

	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)

	go func() {
		defer pipeWriter.Close()
		defer writer.Close()

		part, err := writer.CreateFormFile("music", "large.mp3")
		if err != nil {
			_ = pipeWriter.CloseWithError(err)
			return
		}

		if _, err := io.CopyN(part, zeroReader{}, maxUploadMusicBytes+1); err != nil {
			_ = pipeWriter.CloseWithError(err)
		}
	}()

	req := httptest.NewRequest(http.MethodPost, "/api/upload-music", pipeReader)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()

	handleUploadMusic(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !called {
		t.Fatalf("expected multipart cleanup to be called on parse failure")
	}
	if msg := parseError(t, rr); !strings.Contains(msg, "文件过大") {
		t.Fatalf("unexpected error message: %s", msg)
	}
}

func TestUploadMultipartMemoryLimitIsBounded(t *testing.T) {
	const maxSafeMemory = 8 << 20

	if maxUploadMusicFormMemory > maxSafeMemory {
		t.Fatalf("maxUploadMusicFormMemory=%d exceeds safe bound=%d", maxUploadMusicFormMemory, maxSafeMemory)
	}

	if maxUploadMusicFormMemory >= maxUploadMusicBytes {
		t.Fatalf("maxUploadMusicFormMemory=%d should be lower than maxUploadMusicBytes=%d", maxUploadMusicFormMemory, maxUploadMusicBytes)
	}
}

func TestHandleSynthesizeLoopOmittedKeepsTriStateNil(t *testing.T) {
	tempDir := filepath.Join(os.TempDir(), "ttsbridge_music")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	tempFile, err := os.CreateTemp(tempDir, "music_*.mp3")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	originalBuilder := buildBackgroundMusicOptions
	defer func() { buildBackgroundMusicOptions = originalBuilder }()

	builderCalled := false
	buildBackgroundMusicOptions = func(req *backgroundMusicRequest) (*tts.BackgroundMusicOptions, error) {
		builderCalled = true
		if req == nil {
			t.Fatalf("background music request should not be nil")
		}
		if req.Loop != nil {
			t.Fatalf("expected nil loop when omitted, got %v", *req.Loop)
		}
		return nil, nil
	}

	payload := map[string]interface{}{
		"text":     "hello",
		"voice":    "test-voice",
		"provider": "unknown",
		"background_music": map[string]interface{}{
			"music_path": tempFile.Name(),
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/synthesize", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handleSynthesize(rr, req)

	if !builderCalled {
		t.Fatalf("expected background music builder to be called")
	}
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if msg := parseError(t, rr); !strings.Contains(msg, "不支持的提供商") {
		t.Fatalf("unexpected error message: %s", msg)
	}
}

func TestHandleSynthesizeRejectsUnknownJSONFields(t *testing.T) {
	body := []byte(`{"text":"hello","voice":"test-voice","provider":"unknown","unknown_field":123}`)
	req := httptest.NewRequest(http.MethodPost, "/api/synthesize", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handleSynthesize(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if msg := parseError(t, rr); !strings.Contains(msg, "请求数据格式错误") {
		t.Fatalf("unexpected error message: %s", msg)
	}
}

func TestHandleSynthesizeRejectsTrailingJSON(t *testing.T) {
	body := []byte(`{"text":"hello","voice":"test-voice","provider":"unknown"}{"extra":1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/synthesize", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handleSynthesize(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if msg := parseError(t, rr); !strings.Contains(msg, "请求体只能包含一个 JSON 对象") {
		t.Fatalf("unexpected error message: %s", msg)
	}
}
