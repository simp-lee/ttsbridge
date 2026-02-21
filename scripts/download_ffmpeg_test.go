package scripts_test

// This file is the authoritative integration test suite for scripts/download-ffmpeg.sh.

import (
	"archive/tar"
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type scriptResult struct {
	exitCode int
	output   string
}

func requireFFmpegDownloadIntegration(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping ffmpeg download integration test in short mode")
	}
	if os.Getenv("TTSBRIDGE_RUN_FFMPEG_DOWNLOAD_INTEGRATION") != "1" {
		t.Skip("set TTSBRIDGE_RUN_FFMPEG_DOWNLOAD_INTEGRATION=1 to run real ffmpeg download integration tests")
	}
}

func requireTool(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not found", name)
	}
}

func requireDownloadTool(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("curl"); err == nil {
		return
	}
	t.Skip("curl not found")
}

func insecureCurlHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".curlrc")
	if err := os.WriteFile(configPath, []byte("insecure\n"), 0o644); err != nil {
		t.Fatalf("write curl config failed: %v", err)
	}
	return dir
}

func requireJQ(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("jq"); err == nil {
		return
	}
	t.Skip("jq not found")
}

func failingJQPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	jqPath := filepath.Join(dir, "jq")
	if err := os.WriteFile(jqPath, []byte("#!/usr/bin/env bash\nexit 2\n"), 0o755); err != nil {
		t.Fatalf("write fake jq failed: %v", err)
	}
	return dir + string(os.PathListSeparator) + os.Getenv("PATH")
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(thisFile))
}

func runDownloadScript(t *testing.T, env map[string]string, args ...string) scriptResult {
	t.Helper()
	requireDownloadTool(t)

	root := repoRoot(t)
	scriptPath := filepath.Join(root, "scripts", "download-ffmpeg.sh")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", append([]string{scriptPath}, args...)...)
	cmd.Dir = root
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("script execution timed out: %s", string(out))
	}

	result := scriptResult{output: string(out)}
	if err == nil {
		return result
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.exitCode = exitErr.ExitCode()
		return result
	}

	t.Fatalf("failed to execute script: %v\noutput:\n%s", err, out)
	return scriptResult{}
}

func runDownloadScriptInRoot(t *testing.T, root string, env map[string]string, args ...string) scriptResult {
	t.Helper()
	requireDownloadTool(t)

	scriptPath := filepath.Join(root, "scripts", "download-ffmpeg.sh")

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", append([]string{scriptPath}, args...)...)
	cmd.Dir = root
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("script execution timed out: %s", string(out))
	}

	result := scriptResult{output: string(out)}
	if err == nil {
		return result
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.exitCode = exitErr.ExitCode()
		return result
	}

	t.Fatalf("failed to execute script: %v\noutput:\n%s", err, out)
	return scriptResult{}
}

func prepareTempScriptProject(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	scriptsDir := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("create scripts dir: %v", err)
	}

	sourcePath := filepath.Join(repoRoot(t), "scripts", "download-ffmpeg.sh")
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read source script: %v", err)
	}

	targetPath := filepath.Join(scriptsDir, "download-ffmpeg.sh")
	if err := os.WriteFile(targetPath, content, 0o755); err != nil {
		t.Fatalf("write target script: %v", err)
	}

	return root
}

func createWindowsArchiveFixture(t *testing.T, archivePath string) {
	t.Helper()

	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create windows fixture archive: %v", err)
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	entries := map[string]string{
		"ffmpeg-8.0.1-essentials_build/bin/ffmpeg.exe":  "fixture-ffmpeg-exe",
		"ffmpeg-8.0.1-essentials_build/bin/ffprobe.exe": "fixture-ffprobe-exe",
	}
	for name, content := range entries {
		writer, createErr := zipWriter.Create(name)
		if createErr != nil {
			t.Fatalf("create zip entry %q: %v", name, createErr)
		}
		if _, writeErr := writer.Write([]byte(content)); writeErr != nil {
			t.Fatalf("write zip entry %q: %v", name, writeErr)
		}
	}

	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close windows fixture archive: %v", err)
	}
}

func createLinuxArchiveFixture(t *testing.T, archivePath string) {
	t.Helper()

	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create linux fixture archive: %v", err)
	}
	defer file.Close()

	tarWriter := tar.NewWriter(file)
	entries := []struct {
		name    string
		mode    int64
		content string
	}{
		{
			name: "ffmpeg-n8.0.1-linux64-gpl-8.0/bin/ffmpeg",
			mode: 0o755,
			content: "#!/usr/bin/env bash\n" +
				"echo 'ffmpeg version 8.0.1 fixture'\n",
		},
		{
			name: "ffmpeg-n8.0.1-linux64-gpl-8.0/bin/ffprobe",
			mode: 0o755,
			content: "#!/usr/bin/env bash\n" +
				"echo 'ffprobe version 8.0.1 fixture'\n",
		},
	}

	for _, entry := range entries {
		header := &tar.Header{
			Name: entry.name,
			Mode: entry.mode,
			Size: int64(len(entry.content)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("write tar header %q: %v", entry.name, err)
		}
		if _, err := tarWriter.Write([]byte(entry.content)); err != nil {
			t.Fatalf("write tar entry %q: %v", entry.name, err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close linux fixture archive: %v", err)
	}
}

func createFixtureCurlPath(t *testing.T, fixtureArchivePath string) string {
	t.Helper()

	dir := t.TempDir()
	curlPath := filepath.Join(dir, "curl")
	script := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

dest=""
args=("$@")
for ((idx=0; idx<${#args[@]}; idx++)); do
  if [[ "${args[$idx]}" == "-o" && $((idx+1)) -lt ${#args[@]} ]]; then
    dest="${args[$((idx+1))]}"
    break
  fi
done

if [[ -n "$dest" ]]; then
  cp "%s" "$dest"
fi
`, fixtureArchivePath)

	if err := os.WriteFile(curlPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fixture curl: %v", err)
	}

	return dir + string(os.PathListSeparator) + os.Getenv("PATH")
}

func TestDownloadFFmpegLinuxDryRunUsesOverrideURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/override" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	result := runDownloadScript(t,
		map[string]string{"LINUX_FFMPEG_URL": server.URL + "/override"},
		"--platform", "linux", "--dry-run",
	)

	if result.exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0\noutput:\n%s", result.exitCode, result.output)
	}
	if !strings.Contains(result.output, "Using Linux URL from LINUX_FFMPEG_URL override.") {
		t.Fatalf("expected override log, output:\n%s", result.output)
	}
	if !strings.Contains(result.output, "Dry-run mode: skipping Linux download/extract.") {
		t.Fatalf("expected dry-run skip log, output:\n%s", result.output)
	}
	if strings.Contains(result.output, "Using temp directory:") {
		t.Fatalf("dry-run should not create temp directory, output:\n%s", result.output)
	}
}

func TestDownloadFFmpegLinuxDryRunPrefersExactAssetMatch(t *testing.T) {
	var exactURL string
	var trackURL string

	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		payload := fmt.Sprintf(`[
  {
    "browser_download_url": "%s/releases/download/tag-track/ffmpeg-n8.0-linux64-gpl-8.0.tar.xz"
  },
  {
    "browser_download_url": "%s/releases/download/tag-exact/ffmpeg-n8.0.1-linux64-gpl-8.0.tar.xz"
  }
]`, "https://"+r.Host, "https://"+r.Host)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	})
	mux.HandleFunc("/releases/download/tag-exact/ffmpeg-n8.0.1-linux64-gpl-8.0.tar.xz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/releases/download/tag-track/ffmpeg-n8.0-linux64-gpl-8.0.tar.xz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewTLSServer(mux)
	defer server.Close()

	trackURL = server.URL + "/releases/download/tag-track/ffmpeg-n8.0-linux64-gpl-8.0.tar.xz"
	exactURL = server.URL + "/releases/download/tag-exact/ffmpeg-n8.0.1-linux64-gpl-8.0.tar.xz"

	result := runDownloadScript(t,
		map[string]string{
			"LINUX_RELEASES_API_BASE": server.URL + "/api",
			"LINUX_RELEASES_PAGES":    "1",
			"CURL_HOME":               insecureCurlHome(t),
		},
		"--platform", "linux", "--dry-run",
	)

	if result.exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0\noutput:\n%s", result.exitCode, result.output)
	}
	if !strings.Contains(result.output, "Resolved Linux URL via releases API (exact version match).") {
		t.Fatalf("expected exact-match resolution log, output:\n%s", result.output)
	}
	if !strings.Contains(result.output, "Checking URL reachability: "+exactURL) {
		t.Fatalf("expected exact URL reachability check, output:\n%s", result.output)
	}
	if strings.Contains(result.output, "Checking URL reachability: "+trackURL) {
		t.Fatalf("did not expect track URL to be selected when exact exists, output:\n%s", result.output)
	}
}

func TestDownloadFFmpegLinuxDryRunFallsBackToTrackMatch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		payload := fmt.Sprintf(`[
  {
    "browser_download_url": "%s/releases/download/tag-track/ffmpeg-n8.0-linux64-gpl-8.0.tar.xz"
  }
]`, "https://"+r.Host)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	})
	mux.HandleFunc("/releases/download/tag-track/ffmpeg-n8.0-linux64-gpl-8.0.tar.xz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewTLSServer(mux)
	defer server.Close()

	trackURL := server.URL + "/releases/download/tag-track/ffmpeg-n8.0-linux64-gpl-8.0.tar.xz"
	result := runDownloadScript(t,
		map[string]string{
			"LINUX_RELEASES_API_BASE": server.URL + "/api",
			"LINUX_RELEASES_PAGES":    "1",
			"CURL_HOME":               insecureCurlHome(t),
		},
		"--platform", "linux", "--dry-run",
	)

	if result.exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0\noutput:\n%s", result.exitCode, result.output)
	}
	if !strings.Contains(result.output, "Exact Linux asset not found; using release API track fallback.") {
		t.Fatalf("expected track fallback log, output:\n%s", result.output)
	}
	if !strings.Contains(result.output, "Checking URL reachability: "+trackURL) {
		t.Fatalf("expected track URL reachability check, output:\n%s", result.output)
	}
}

func TestDownloadFFmpegLinuxDryRunPrefersJQParserWhenAvailable(t *testing.T) {
	requireJQ(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		payload := fmt.Sprintf(`[
  {
    "assets": [
      {
        "browser_download_url": "%s/releases/download/tag-track/ffmpeg-n8.0-linux64-gpl-8.0.tar.xz"
      },
      {
        "browser_download_url": "%s/releases/download/tag-exact/ffmpeg-n8.0.1-linux64-gpl-8.0.tar.xz"
      }
    ]
  }
]`, "https://"+r.Host, "https://"+r.Host)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	})
	mux.HandleFunc("/releases/download/tag-exact/ffmpeg-n8.0.1-linux64-gpl-8.0.tar.xz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewTLSServer(mux)
	defer server.Close()

	exactURL := server.URL + "/releases/download/tag-exact/ffmpeg-n8.0.1-linux64-gpl-8.0.tar.xz"
	result := runDownloadScript(t,
		map[string]string{
			"LINUX_RELEASES_API_BASE": server.URL + "/api",
			"LINUX_RELEASES_PAGES":    "1",
			"CURL_HOME":               insecureCurlHome(t),
		},
		"--platform", "linux", "--dry-run",
	)

	if result.exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0\noutput:\n%s", result.exitCode, result.output)
	}
	if !strings.Contains(result.output, "Parsed Linux release metadata with jq") {
		t.Fatalf("expected jq parser log, output:\n%s", result.output)
	}
	if !strings.Contains(result.output, "Checking URL reachability: "+exactURL) {
		t.Fatalf("expected exact URL reachability check, output:\n%s", result.output)
	}
}

func TestDownloadFFmpegLinuxDryRunFallsBackWhenJQParsingFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		payload := fmt.Sprintf(`[
  {
    "browser_download_url": "%s/releases/download/tag-exact/ffmpeg-n8.0.1-linux64-gpl-8.0.tar.xz"
  }
]`, "https://"+r.Host)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	})
	mux.HandleFunc("/releases/download/tag-exact/ffmpeg-n8.0.1-linux64-gpl-8.0.tar.xz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewTLSServer(mux)
	defer server.Close()

	exactURL := server.URL + "/releases/download/tag-exact/ffmpeg-n8.0.1-linux64-gpl-8.0.tar.xz"
	result := runDownloadScript(t,
		map[string]string{
			"LINUX_RELEASES_API_BASE": server.URL + "/api",
			"LINUX_RELEASES_PAGES":    "1",
			"CURL_HOME":               insecureCurlHome(t),
			"PATH":                    failingJQPath(t),
		},
		"--platform", "linux", "--dry-run",
	)

	if result.exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0\noutput:\n%s", result.exitCode, result.output)
	}
	if !strings.Contains(result.output, "jq parser returned no release URLs; falling back to grep/sed") {
		t.Fatalf("expected jq fallback log, output:\n%s", result.output)
	}
	if !strings.Contains(result.output, "Checking URL reachability: "+exactURL) {
		t.Fatalf("expected exact URL reachability check after fallback, output:\n%s", result.output)
	}
}

func TestDownloadFFmpegLinuxFailsWhenHashPlaceholderUsed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/archive.tar.xz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-a-real-archive"))
	}))
	defer server.Close()

	result := runDownloadScript(t,
		map[string]string{"LINUX_FFMPEG_URL": server.URL + "/archive.tar.xz"},
		"--platform", "linux",
	)

	if result.exitCode == 0 {
		t.Fatalf("expected non-zero exit code for placeholder hash failure, output:\n%s", result.output)
	}
	if !strings.Contains(result.output, "linux-tar.xz hash is still a placeholder") {
		t.Fatalf("expected placeholder-hash failure message, output:\n%s", result.output)
	}
}

func TestFFmpegDownloadDocs_CommandsMatchScriptHelp(t *testing.T) {
	readmePath := filepath.Join(repoRoot(t), "README.md")
	content, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README.md failed: %v", err)
	}
	readme := string(content)

	requiredCommands := []string{
		"./scripts/download-ffmpeg.sh --platform linux --dry-run",
		"./scripts/download-ffmpeg.sh              # 自动检测当前平台",
		"./scripts/download-ffmpeg.sh --platform linux",
		"./scripts/download-ffmpeg.sh --platform windows",
		"./scripts/download-ffmpeg.sh --all",
		"./scripts/download-ffmpeg.sh --platform linux --skip-verify",
	}
	for _, command := range requiredCommands {
		if !strings.Contains(readme, command) {
			t.Fatalf("README should document command: %q", command)
		}
	}

	result := runDownloadScript(t, nil, "--help")
	if result.exitCode != 0 {
		t.Fatalf("--help should exit 0, got %d\noutput:\n%s", result.exitCode, result.output)
	}

	requiredFlags := []string{"--platform", "--all", "--dry-run", "--skip-verify"}
	for _, flag := range requiredFlags {
		if !strings.Contains(result.output, flag) {
			t.Fatalf("script help should include %s, output:\n%s", flag, result.output)
		}
	}
}

func TestDownloadAndExtractFFmpegWindows_FixtureIntegration(t *testing.T) {
	requireTool(t, "unzip")

	archivePath := filepath.Join(t.TempDir(), "windows-ffmpeg-fixture.zip")
	createWindowsArchiveFixture(t, archivePath)

	root := prepareTempScriptProject(t)
	result := runDownloadScriptInRoot(t, root, map[string]string{
		"PATH": createFixtureCurlPath(t, archivePath),
	}, "--platform", "windows", "--skip-verify")

	if result.exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0\noutput:\n%s", result.exitCode, result.output)
	}

	ffmpegPath := filepath.Join(root, "ffmpeg", "bin", "ffmpeg.exe")
	ffprobePath := filepath.Join(root, "ffmpeg", "bin", "ffprobe.exe")
	for _, path := range []string{ffmpegPath, ffprobePath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected extracted binary %s: %v", path, err)
		}
		if info.Size() == 0 {
			t.Fatalf("extracted binary %s is empty", path)
		}
	}
}

func TestDownloadAndExtractFFmpegLinux_FixtureIntegration(t *testing.T) {
	requireTool(t, "tar")

	archivePath := filepath.Join(t.TempDir(), "linux-ffmpeg-fixture.tar.xz")
	createLinuxArchiveFixture(t, archivePath)

	root := prepareTempScriptProject(t)
	result := runDownloadScriptInRoot(t, root, map[string]string{
		"PATH":             createFixtureCurlPath(t, archivePath),
		"LINUX_FFMPEG_URL": "https://fixture.local/ffmpeg-linux.tar.xz",
	}, "--platform", "linux", "--skip-verify")

	if result.exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0\noutput:\n%s", result.exitCode, result.output)
	}

	ffmpegPath := filepath.Join(root, "ffmpeg", "bin", "ffmpeg")
	ffprobePath := filepath.Join(root, "ffmpeg", "bin", "ffprobe")
	for _, path := range []string{ffmpegPath, ffprobePath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected extracted binary %s: %v", path, err)
		}
		if info.Size() == 0 {
			t.Fatalf("extracted binary %s is empty", path)
		}
		if info.Mode()&0o111 == 0 {
			t.Fatalf("expected executable permission on %s, mode=%v", path, info.Mode())
		}
	}

	versionOut, err := exec.Command(ffmpegPath, "-version").CombinedOutput()
	if err != nil {
		t.Fatalf("run extracted fixture ffmpeg -version: %v\n%s", err, versionOut)
	}
	if !strings.Contains(string(versionOut), "ffmpeg version 8.0.1") {
		t.Fatalf("unexpected fixture ffmpeg version output:\n%s", versionOut)
	}
}

func TestDownloadAndExtractFFmpegWindows_Integration(t *testing.T) {
	t.Run("HappyPath_ExtractsBinaries", func(t *testing.T) {
		requireFFmpegDownloadIntegration(t)
		requireTool(t, "unzip")

		root := prepareTempScriptProject(t)
		result := runDownloadScriptInRoot(t, root, nil, "--platform", "windows", "--skip-verify")

		if result.exitCode != 0 {
			t.Fatalf("exitCode=%d, want 0\noutput:\n%s", result.exitCode, result.output)
		}
		if !strings.Contains(result.output, "ffmpeg-8.0.1-essentials_build.zip") {
			t.Fatalf("expected pinned windows 8.0.1 asset URL in output, output:\n%s", result.output)
		}

		ffmpegPath := filepath.Join(root, "ffmpeg", "bin", "ffmpeg.exe")
		ffprobePath := filepath.Join(root, "ffmpeg", "bin", "ffprobe.exe")
		for _, p := range []string{ffmpegPath, ffprobePath} {
			info, err := os.Stat(p)
			if err != nil {
				t.Fatalf("expected extracted binary %s: %v", p, err)
			}
			if info.Size() == 0 {
				t.Fatalf("extracted binary %s is empty", p)
			}
		}
	})

	t.Run("ErrorPath_MissingDownloadTool", func(t *testing.T) {
		root := prepareTempScriptProject(t)
		result := runDownloadScriptInRoot(t, root, map[string]string{"PATH": ""}, "--platform", "windows", "--dry-run")

		if result.exitCode == 0 {
			t.Fatalf("expected non-zero exit code when neither curl nor wget exists, output:\n%s", result.output)
		}
		if !strings.Contains(result.output, "Neither curl nor wget found") {
			t.Fatalf("expected missing downloader error, output:\n%s", result.output)
		}
	})
}

func TestDownloadAndExtractFFmpegLinux_Integration(t *testing.T) {
	t.Run("HappyPath_ExtractsAndIsExecutable", func(t *testing.T) {
		requireFFmpegDownloadIntegration(t)
		requireTool(t, "tar")

		root := prepareTempScriptProject(t)
		env := map[string]string{"LINUX_FFMPEG_URL": "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-n8.0.1-linux64-gpl-8.0.tar.xz"}
		result := runDownloadScriptInRoot(t, root, env, "--platform", "linux", "--skip-verify")

		if result.exitCode != 0 {
			t.Fatalf("exitCode=%d, want 0\noutput:\n%s", result.exitCode, result.output)
		}

		ffmpegPath := filepath.Join(root, "ffmpeg", "bin", "ffmpeg")
		ffprobePath := filepath.Join(root, "ffmpeg", "bin", "ffprobe")
		for _, p := range []string{ffmpegPath, ffprobePath} {
			info, err := os.Stat(p)
			if err != nil {
				t.Fatalf("expected extracted binary %s: %v", p, err)
			}
			if info.Size() == 0 {
				t.Fatalf("extracted binary %s is empty", p)
			}
			if info.Mode()&0o111 == 0 {
				t.Fatalf("expected executable permission on %s, mode=%v", p, info.Mode())
			}
		}

		cmd := exec.Command(ffmpegPath, "-version")
		versionOut, err := cmd.Output()
		if err != nil {
			t.Fatalf("run extracted ffmpeg -version: %v", err)
		}
		if !strings.Contains(string(versionOut), "ffmpeg version") || !strings.Contains(string(versionOut), "8.0.1") {
			t.Fatalf("expected ffmpeg version output with 8.0.1, got:\n%s", string(versionOut))
		}
	})

	t.Run("ErrorPath_HashVerificationFailsWithoutSkipVerify", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/archive.tar.xz" {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "not-a-real-archive")
		}))
		defer server.Close()

		root := prepareTempScriptProject(t)
		result := runDownloadScriptInRoot(t, root, map[string]string{"LINUX_FFMPEG_URL": server.URL + "/archive.tar.xz"}, "--platform", "linux")

		if result.exitCode == 0 {
			t.Fatalf("expected non-zero exit code for placeholder hash verification failure, output:\n%s", result.output)
		}
		if !strings.Contains(result.output, "linux-tar.xz hash is still a placeholder") {
			t.Fatalf("expected placeholder hash failure message, output:\n%s", result.output)
		}
	})
}
