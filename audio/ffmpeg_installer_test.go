package audio

import (
	"archive/zip"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFFmpegBinaryName(t *testing.T) {
	name := ffmpegBinaryName()
	if runtime.GOOS == "windows" {
		if name != "ffmpeg.exe" {
			t.Errorf("ffmpegBinaryName() = %q; want %q on windows", name, "ffmpeg.exe")
		}
	} else {
		if name != "ffmpeg" {
			t.Errorf("ffmpegBinaryName() = %q; want %q on %s", name, "ffmpeg", runtime.GOOS)
		}
	}
}

func TestFFprobeBinaryName(t *testing.T) {
	name := ffprobeBinaryName()
	if runtime.GOOS == "windows" {
		if name != "ffprobe.exe" {
			t.Errorf("ffprobeBinaryName() = %q; want %q on windows", name, "ffprobe.exe")
		}
	} else {
		if name != "ffprobe" {
			t.Errorf("ffprobeBinaryName() = %q; want %q on %s", name, "ffprobe", runtime.GOOS)
		}
	}
}

func TestGetLibraryFFmpegDir(t *testing.T) {
	dir := getLibraryFFmpegDir()
	if dir == "" {
		t.Fatal("getLibraryFFmpegDir() returned empty string")
	}

	// 应该以 ffmpeg/bin 结尾
	if filepath.Base(dir) != "bin" {
		t.Errorf("getLibraryFFmpegDir() = %q; want path ending with 'bin'", dir)
	}
	parent := filepath.Base(filepath.Dir(dir))
	if parent != "ffmpeg" {
		t.Errorf("getLibraryFFmpegDir() parent = %q; want 'ffmpeg'", parent)
	}

	// 目录应该存在（库中有 ffmpeg/bin/ 目录）
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("getLibraryFFmpegDir() path %q does not exist", dir)
	}
}

func TestIsFFmpegInstalled_SearchOrder(t *testing.T) {
	tmpDir := t.TempDir()
	binaryName := ffmpegBinaryName()
	binaryPath := filepath.Join(tmpDir, binaryName)
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write fake ffmpeg binary: %v", err)
	}

	originalPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmpDir); err != nil {
		t.Fatalf("set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", originalPath)
	})

	if !IsFFmpegInstalled() {
		t.Error("IsFFmpegInstalled() = false; want true when ffmpeg exists in PATH")
	}

	resolved, found := findBinary(binaryName)
	if !found {
		t.Fatal("findBinary() should find ffmpeg in PATH")
	}
	if filepath.Clean(resolved) != filepath.Clean(binaryPath) {
		t.Errorf("findBinary() = %q; want %q", resolved, binaryPath)
	}
}

func TestFindBinary_ReturnsPath(t *testing.T) {
	// findBinary 应该对系统上存在的工具（如 ls/cmd）返回非空路径
	var knownBin string
	if runtime.GOOS == "windows" {
		knownBin = "cmd.exe"
	} else {
		knownBin = "ls"
	}

	path, found := findBinary(knownBin)
	if !found {
		t.Skipf("%s not found on this system, skipping", knownBin)
	}
	if path == "" {
		t.Errorf("findBinary(%q) returned empty path but found=true", knownBin)
	}
}

func TestFindBinary_NotFound(t *testing.T) {
	path, found := findBinary("definitely_nonexistent_binary_xyz")
	if found {
		t.Errorf("findBinary returned found=true for nonexistent binary, path=%q", path)
	}
	if path != "" {
		t.Errorf("findBinary returned non-empty path %q for nonexistent binary", path)
	}
}

func TestIsFFprobeInstalled_SearchOrder(t *testing.T) {
	tmpDir := t.TempDir()
	binaryName := ffprobeBinaryName()
	binaryPath := filepath.Join(tmpDir, binaryName)
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write fake ffprobe binary: %v", err)
	}

	originalPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmpDir); err != nil {
		t.Fatalf("set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", originalPath)
	})

	if !IsFFprobeInstalled() {
		t.Error("IsFFprobeInstalled() = false; want true when ffprobe exists in PATH")
	}

	resolved, found := findBinary(binaryName)
	if !found {
		t.Fatal("findBinary() should find ffprobe in PATH")
	}
	if filepath.Clean(resolved) != filepath.Clean(binaryPath) {
		t.Errorf("findBinary() = %q; want %q", resolved, binaryPath)
	}
}

func TestIsFFprobeInstalled_LibraryDirFallback(t *testing.T) {
	libDir := getLibraryFFmpegDir()
	if libDir == "" {
		t.Fatal("getLibraryFFmpegDir() returned empty string")
	}

	binaryName := ffprobeBinaryName()
	targetPath := filepath.Join(libDir, binaryName)

	originalData, readErr := os.ReadFile(targetPath)
	originalExists := readErr == nil
	var originalMode os.FileMode
	if originalExists {
		info, err := os.Stat(targetPath)
		if err != nil {
			t.Fatalf("stat original ffprobe: %v", err)
		}
		originalMode = info.Mode()
	}

	fixtureContent := []byte("#!/usr/bin/env bash\necho 'ffprobe fixture'\n")
	if runtime.GOOS == "windows" {
		fixtureContent = []byte("@echo off\r\necho ffprobe fixture\r\n")
	}
	if err := os.WriteFile(targetPath, fixtureContent, 0755); err != nil {
		t.Fatalf("write fixture ffprobe in library dir: %v", err)
	}
	t.Cleanup(func() {
		if originalExists {
			_ = os.WriteFile(targetPath, originalData, originalMode)
			return
		}
		_ = os.Remove(targetPath)
	})

	if installDir, err := getInstallDir(); err == nil {
		installPath := filepath.Join(installDir, "bin", binaryName)
		if filepath.Clean(installPath) != filepath.Clean(targetPath) {
			if _, statErr := os.Stat(installPath); statErr == nil {
				bakPath := installPath + ".bak_test"
				if renameErr := os.Rename(installPath, bakPath); renameErr != nil {
					t.Fatalf("temporarily move install-dir ffprobe: %v", renameErr)
				}
				t.Cleanup(func() {
					_ = os.Rename(bakPath, installPath)
				})
			}
		}
	}

	originalPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", t.TempDir()); err != nil {
		t.Fatalf("set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", originalPath)
	})

	resolved, found := findBinary(binaryName)
	if !found {
		t.Fatalf("findBinary(%q) should find fixture in library dir", binaryName)
	}
	if filepath.Clean(resolved) != filepath.Clean(targetPath) {
		t.Fatalf("findBinary(%q) = %q; want %q (library dir fallback)", binaryName, resolved, targetPath)
	}

	if !IsFFprobeInstalled() {
		t.Fatal("IsFFprobeInstalled() = false; want true via library-dir fallback")
	}
}

// lookPathInSystem 是一个测试辅助函数，检查系统 PATH 中是否有指定二进制
func lookPathInSystem(name string) bool {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// requireFFmpeg 是一个测试辅助函数，如果 ffmpeg 不可用则跳过测试
func requireFFmpeg(t *testing.T) string {
	t.Helper()
	path, found := findBinary(ffmpegBinaryName())
	if !found {
		t.Skip("ffmpeg not available, skipping")
	}
	return path
}

// --- 集成测试 ---

func TestFFmpegIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ffmpegPath := requireFFmpeg(t)

	t.Run("GenerateSilence", func(t *testing.T) {
		tmpDir := t.TempDir()
		outFile := filepath.Join(tmpDir, "silence.wav")

		// 使用 ffmpeg 生成 0.5 秒静音 WAV 文件
		cmd := exec.Command(ffmpegPath,
			"-f", "lavfi",
			"-i", "anullsrc=r=24000:cl=mono",
			"-t", "0.5",
			"-c:a", "pcm_s16le",
			outFile,
			"-y",
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("ffmpeg failed to generate silence: %v\noutput: %s", err, output)
		}

		// 验证输出文件存在且非空
		info, err := os.Stat(outFile)
		if err != nil {
			t.Fatalf("output file does not exist: %v", err)
		}
		if info.Size() == 0 {
			t.Error("output file is empty")
		}
		// 0.5 秒 24kHz mono 16-bit WAV ≈ 24000 bytes + header
		if info.Size() < 1000 {
			t.Errorf("output file unexpectedly small: %d bytes", info.Size())
		}
	})

	t.Run("VersionOutput", func(t *testing.T) {
		// 验证 ffmpeg 能正常输出版本信息
		cmd := exec.Command(ffmpegPath, "-version")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("ffmpeg -version failed: %v\noutput: %s", err, output)
		}
		if !strings.Contains(string(output), "ffmpeg version") {
			t.Errorf("unexpected version output: %s", output)
		}
	})

	t.Run("GenerateMP3", func(t *testing.T) {
		tmpDir := t.TempDir()
		outFile := filepath.Join(tmpDir, "silence.mp3")

		cmd := exec.Command(ffmpegPath,
			"-f", "lavfi",
			"-i", "anullsrc=r=24000:cl=mono",
			"-t", "0.5",
			"-c:a", "libmp3lame",
			"-b:a", "128k",
			outFile,
			"-y",
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Skipf("ffmpeg mp3 encoding not supported (missing libmp3lame?): %v\n%s", err, output)
		}

		info, err := os.Stat(outFile)
		if err != nil {
			t.Fatalf("mp3 output file does not exist: %v", err)
		}
		if info.Size() == 0 {
			t.Error("mp3 output file is empty")
		}
	})
}

func TestFFmpegIntegration_WithFixtureBinary(t *testing.T) {
	tmpDir := t.TempDir()
	binaryName := ffmpegBinaryName()
	fixtureFFmpegPath := filepath.Join(tmpDir, binaryName)

	fixtureScript := "#!/usr/bin/env bash\n" +
		"set -euo pipefail\n" +
		"if [[ ${1:-} == '-version' ]]; then\n" +
		"  echo 'ffmpeg version 8.0.1 fixture'\n" +
		"  exit 0\n" +
		"fi\n" +
		"out=''\n" +
		"for arg in \"$@\"; do\n" +
		"  if [[ $arg == *.wav || $arg == *.mp3 ]]; then\n" +
		"    out=$arg\n" +
		"  fi\n" +
		"done\n" +
		"if [[ -z $out ]]; then\n" +
		"  echo 'missing output path' >&2\n" +
		"  exit 2\n" +
		"fi\n" +
		"python3 - <<'PY' \"$out\"\n" +
		"import sys\n" +
		"with open(sys.argv[1], 'wb') as f:\n" +
		"    f.write(b'RIFF')\n" +
		"    f.write(b'\\x00' * 2048)\n" +
		"PY\n"

	if err := os.WriteFile(fixtureFFmpegPath, []byte(fixtureScript), 0755); err != nil {
		t.Fatalf("write fixture ffmpeg: %v", err)
	}

	t.Run("GenerateSilenceWithFixture", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "fixture-silence.wav")
		cmd := exec.Command(fixtureFFmpegPath,
			"-f", "lavfi",
			"-i", "anullsrc=r=24000:cl=mono",
			"-t", "0.5",
			"-c:a", "pcm_s16le",
			outFile,
			"-y",
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture ffmpeg failed: %v\n%s", err, output)
		}

		info, err := os.Stat(outFile)
		if err != nil {
			t.Fatalf("fixture output missing: %v", err)
		}
		if info.Size() < 1024 {
			t.Fatalf("fixture output unexpectedly small: %d bytes", info.Size())
		}
	})

	t.Run("VersionOutputWithFixture", func(t *testing.T) {
		output, err := exec.Command(fixtureFFmpegPath, "-version").CombinedOutput()
		if err != nil {
			t.Fatalf("fixture ffmpeg -version failed: %v\n%s", err, output)
		}
		if !strings.Contains(string(output), "ffmpeg version 8.0.1") {
			t.Fatalf("unexpected fixture version output: %s", output)
		}
	})

	t.Run("DeterministicFixtureOutputSignature", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "fixture-signature.wav")
		output, err := exec.Command(fixtureFFmpegPath,
			"-f", "lavfi",
			"-i", "anullsrc=r=24000:cl=mono",
			"-t", "0.5",
			"-c:a", "pcm_s16le",
			outFile,
			"-y",
		).CombinedOutput()
		if err != nil {
			t.Fatalf("fixture ffmpeg deterministic run failed: %v\n%s", err, output)
		}

		payload, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("read fixture output failed: %v", err)
		}
		if len(payload) != 2052 {
			t.Fatalf("fixture output size = %d; want %d", len(payload), 2052)
		}
		if string(payload[:4]) != "RIFF" {
			t.Fatalf("fixture output header = %q; want %q", string(payload[:4]), "RIFF")
		}
	})
}

func TestFFprobeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ffmpegPath := requireFFmpeg(t)

	probePath, found := findBinary(ffprobeBinaryName())
	if !found {
		t.Skip("ffprobe not available, skipping")
	}

	// 先用 ffmpeg 生成一个测试文件
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.wav")
	cmd := exec.Command(ffmpegPath,
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo",
		"-t", "0.2", "-c:a", "pcm_s16le", testFile, "-y",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to generate test file: %v\n%s", err, out)
	}

	// 用 ffprobe 检查该文件
	cmd = exec.Command(probePath,
		"-v", "error",
		"-show_entries", "stream=codec_name,sample_rate,channels",
		"-of", "default=noprint_wrappers=1",
		testFile,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe failed: %v\noutput: %s", err, output)
	}

	outStr := string(output)
	if !strings.Contains(outStr, "pcm_s16le") {
		t.Errorf("ffprobe did not detect pcm_s16le codec in output: %s", outStr)
	}
	if !strings.Contains(outStr, "sample_rate=44100") {
		t.Errorf("ffprobe did not detect sample_rate=44100 in output: %s", outStr)
	}
}

// --- findBinary 补充测试 ---

func TestFindBinary_LibraryDir(t *testing.T) {
	libDir := getLibraryFFmpegDir()
	if libDir == "" {
		t.Fatal("getLibraryFFmpegDir() returned empty string")
	}
	binaryName := "ttsbridge-findbinary-libtest-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(libDir, binaryName)
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write test binary in library dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(binaryPath)
	})

	originalPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", t.TempDir()); err != nil {
		t.Fatalf("set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", originalPath)
	})

	path, found := findBinary(binaryName)
	if !found {
		t.Errorf("findBinary(%q) should find test binary from library dir", binaryName)
	}
	if path == "" {
		t.Error("findBinary() returned empty path but found=true")
	}
	if filepath.Clean(path) != filepath.Clean(binaryPath) {
		t.Errorf("findBinary() = %q; want %q", path, binaryPath)
	}
}

func TestFindBinary_SystemPATHPreferred(t *testing.T) {
	// 如果系统 PATH 中有该二进制，findBinary 应该返回 PATH 中的版本
	name := ffmpegBinaryName()
	if !lookPathInSystem(name) {
		t.Skip("ffmpeg not in system PATH, cannot test preference")
	}

	path, found := findBinary(name)
	if !found {
		t.Fatal("findBinary() did not find ffmpeg despite being in PATH")
	}

	// 解析 exec.LookPath 的结果来比较
	expectedPath, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("exec.LookPath failed unexpectedly: %v", err)
	}

	if path != expectedPath {
		t.Errorf("findBinary() = %q; want %q (system PATH should be preferred)", path, expectedPath)
	}
}

// --- addToPath 测试 ---

func TestAddToPath(t *testing.T) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath) // 恢复原始 PATH

	testDir := "/tmp/test_ffmpeg_path_dir_12345"

	// 首次添加
	err := addToPath(testDir)
	if err != nil {
		t.Fatalf("addToPath() error: %v", err)
	}

	newPath := os.Getenv("PATH")
	if !strings.Contains(newPath, testDir) {
		t.Errorf("PATH should contain %q after addToPath, got: %s", testDir, newPath)
	}

	// 重复添加应该是幂等的（不会重复）
	err = addToPath(testDir)
	if err != nil {
		t.Fatalf("addToPath() second call error: %v", err)
	}

	finalPath := os.Getenv("PATH")
	count := strings.Count(finalPath, testDir)
	if count != 1 {
		t.Errorf("addToPath() is not idempotent: %q appears %d times in PATH", testDir, count)
	}
}

func TestAddToPath_SubstringPathNotTreatedAsExisting(t *testing.T) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	dir := filepath.Join(string(os.PathSeparator), "tmp", "ffmpeg", "bin")
	substringOnly := dir + "-extra"
	seedPath := strings.Join([]string{substringOnly, filepath.Join(string(os.PathSeparator), "usr", "local", "bin")}, string(os.PathListSeparator))
	if err := os.Setenv("PATH", seedPath); err != nil {
		t.Fatalf("failed to seed PATH: %v", err)
	}

	if err := addToPath(dir); err != nil {
		t.Fatalf("addToPath() error: %v", err)
	}

	entries := filepath.SplitList(os.Getenv("PATH"))
	if len(entries) < 1 {
		t.Fatalf("PATH entries should not be empty")
	}
	if entries[0] != dir {
		t.Fatalf("expected first PATH entry %q, got %q", dir, entries[0])
	}

	matchCount := 0
	for _, entry := range entries {
		if filepath.Clean(entry) == filepath.Clean(dir) {
			matchCount++
		}
	}
	if matchCount != 1 {
		t.Fatalf("expected exact PATH entry %q exactly once, got %d", dir, matchCount)
	}
}

// --- getInstallDir 测试 ---

func TestGetInstallDir(t *testing.T) {
	dir, err := getInstallDir()
	if err != nil {
		t.Fatalf("getInstallDir() error: %v", err)
	}
	if dir == "" {
		t.Fatal("getInstallDir() returned empty string")
	}

	// 应该以 "ffmpeg" 结尾
	if filepath.Base(dir) != "ffmpeg" {
		t.Errorf("getInstallDir() = %q; want path ending with 'ffmpeg'", dir)
	}

	// 父目录应该是可执行文件所在的目录
	exePath, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error: %v", err)
	}
	expectedDir := filepath.Join(filepath.Dir(exePath), "ffmpeg")
	if dir != expectedDir {
		t.Errorf("getInstallDir() = %q; want %q", dir, expectedDir)
	}
}

// --- validateZipFile 测试 ---

func TestValidateZipFile(t *testing.T) {
	t.Run("ValidZip", func(t *testing.T) {
		tmpDir := t.TempDir()
		zipPath := filepath.Join(tmpDir, "valid.zip")

		// 创建一个含有文件的有效 zip
		f, err := os.Create(zipPath)
		if err != nil {
			t.Fatalf("create zip failed: %v", err)
		}
		w := zip.NewWriter(f)
		fw, err := w.Create("test.txt")
		if err != nil {
			t.Fatalf("zip create entry failed: %v", err)
		}
		fw.Write([]byte("hello"))
		w.Close()
		f.Close()

		if err := validateZipFile(zipPath); err != nil {
			t.Errorf("validateZipFile() returned error for valid zip: %v", err)
		}
	})

	t.Run("EmptyZip", func(t *testing.T) {
		tmpDir := t.TempDir()
		zipPath := filepath.Join(tmpDir, "empty.zip")

		// 创建一个空的 zip（没有任何条目）
		f, err := os.Create(zipPath)
		if err != nil {
			t.Fatalf("create zip failed: %v", err)
		}
		w := zip.NewWriter(f)
		w.Close()
		f.Close()

		if err := validateZipFile(zipPath); err == nil {
			t.Error("validateZipFile() should return error for empty zip")
		}
	})

	t.Run("CorruptedFile", func(t *testing.T) {
		tmpDir := t.TempDir()
		zipPath := filepath.Join(tmpDir, "corrupt.zip")

		// 创建一个非 zip 格式的文件
		if err := os.WriteFile(zipPath, []byte("this is not a zip file"), 0644); err != nil {
			t.Fatalf("create file failed: %v", err)
		}

		if err := validateZipFile(zipPath); err == nil {
			t.Error("validateZipFile() should return error for corrupted file")
		}
	})

	t.Run("NonexistentFile", func(t *testing.T) {
		if err := validateZipFile("/nonexistent/path/file.zip"); err == nil {
			t.Error("validateZipFile() should return error for nonexistent file")
		}
	})
}

func TestVerifyFileSHA256(t *testing.T) {
	t.Run("Match", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "payload.bin")
		if err := os.WriteFile(file, []byte("hello"), 0644); err != nil {
			t.Fatalf("write file failed: %v", err)
		}

		err := verifyFileSHA256(file, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
		if err != nil {
			t.Fatalf("verifyFileSHA256 should pass: %v", err)
		}
	})

	t.Run("Mismatch", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "payload.bin")
		if err := os.WriteFile(file, []byte("hello"), 0644); err != nil {
			t.Fatalf("write file failed: %v", err)
		}

		err := verifyFileSHA256(file, "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
		if err == nil {
			t.Fatal("verifyFileSHA256 should fail for mismatched hash")
		}
	})
}

func TestFFmpegWindowsDownloadSources_HashConfig(t *testing.T) {
	t.Run("NoConfiguredHash", func(t *testing.T) {
		t.Setenv("TTSBRIDGE_FFMPEG_OFFICIAL_SHA256", "")
		t.Setenv("TTSBRIDGE_FFMPEG_GITHUB_SHA256", "")

		sources, err := ffmpegWindowsDownloadSources()
		if err == nil {
			t.Fatalf("expected error when no trusted hash is configured, got sources=%v", sources)
		}
	})

	t.Run("ConfiguredOfficialHash", func(t *testing.T) {
		t.Setenv("TTSBRIDGE_FFMPEG_OFFICIAL_SHA256", "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
		t.Setenv("TTSBRIDGE_FFMPEG_GITHUB_SHA256", "")

		sources, err := ffmpegWindowsDownloadSources()
		if err != nil {
			t.Fatalf("ffmpegWindowsDownloadSources() unexpected error: %v", err)
		}
		if len(sources) == 0 {
			t.Fatal("expected at least one trusted source")
		}

		if sources[0].url == "" || sources[0].hash == "" {
			t.Fatalf("source should contain url/hash, got: %+v", sources[0])
		}
	})
}

// --- ffmpegBinaryName / ffprobeBinaryName 补充覆盖测试 ---

func TestBinaryNames_Consistency(t *testing.T) {
	ffmpegName := ffmpegBinaryName()
	ffprobeName := ffprobeBinaryName()

	// 两者扩展名应该一致
	ffmpegExt := filepath.Ext(ffmpegName)
	ffprobeExt := filepath.Ext(ffprobeName)
	if ffmpegExt != ffprobeExt {
		t.Errorf("binary extensions inconsistent: ffmpeg=%q, ffprobe=%q", ffmpegExt, ffprobeExt)
	}

	// 验证名称包含正确的基础字符串
	if !strings.Contains(ffmpegName, "ffmpeg") {
		t.Errorf("ffmpegBinaryName() = %q; should contain 'ffmpeg'", ffmpegName)
	}
	if !strings.Contains(ffprobeName, "ffprobe") {
		t.Errorf("ffprobeBinaryName() = %q; should contain 'ffprobe'", ffprobeName)
	}

	if runtime.GOOS == "windows" {
		if ffmpegExt != ".exe" {
			t.Errorf("on windows, binary extension should be .exe, got %q", ffmpegExt)
		}
	} else {
		if ffmpegExt != "" {
			t.Errorf("on %s, binary extension should be empty, got %q", runtime.GOOS, ffmpegExt)
		}
	}
}

func TestGetLibraryFFmpegDir_Stability(t *testing.T) {
	// 多次调用应该返回相同结果
	dir1 := getLibraryFFmpegDir()
	dir2 := getLibraryFFmpegDir()
	if dir1 != dir2 {
		t.Errorf("getLibraryFFmpegDir() not stable: %q != %q", dir1, dir2)
	}

	// 路径应该是绝对路径
	if !filepath.IsAbs(dir1) {
		t.Errorf("getLibraryFFmpegDir() = %q; want absolute path", dir1)
	}
}

func TestDownloadFileTimeout(t *testing.T) {
	oldTimeout := ffmpegDownloadTimeout
	ffmpegDownloadTimeout = 100 * time.Millisecond
	t.Cleanup(func() {
		ffmpegDownloadTimeout = oldTimeout
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	outputPath := filepath.Join(t.TempDir(), "ffmpeg.zip")
	err := downloadFile(outputPath, server.URL)
	if err == nil {
		t.Fatal("downloadFile should fail when request exceeds timeout")
	}

	errText := strings.ToLower(err.Error())
	if !strings.Contains(errText, "timeout") && !strings.Contains(errText, "deadline exceeded") {
		t.Fatalf("unexpected error for timeout case: %v", err)
	}
}

func TestExtractFFmpeg_DoesNotExtractFFplay(t *testing.T) {
	t.Run("ignores_ffplay_and_extracts_required_binaries", func(t *testing.T) {
		zipPath := filepath.Join(t.TempDir(), "ffmpeg-fixture.zip")
		ffplayName := "ffplay"
		if runtime.GOOS == "windows" {
			ffplayName = "ffplay.exe"
		}

		createZipFixture(t, zipPath, map[string]string{
			"ffmpeg-8.0/bin/" + ffmpegBinaryName():  "ffmpeg binary",
			"ffmpeg-8.0/bin/" + ffprobeBinaryName(): "ffprobe binary",
			"ffmpeg-8.0/bin/" + ffplayName:          "ffplay binary",
		})

		destDir := t.TempDir()
		if err := extractFFmpeg(zipPath, destDir); err != nil {
			t.Fatalf("extractFFmpeg() unexpected error: %v", err)
		}

		mustExist := []string{
			filepath.Join(destDir, "bin", ffmpegBinaryName()),
			filepath.Join(destDir, "bin", ffprobeBinaryName()),
		}
		for _, file := range mustExist {
			if _, err := os.Stat(file); err != nil {
				t.Fatalf("expected extracted file %q to exist: %v", file, err)
			}
		}

		mustNotExist := []string{
			filepath.Join(destDir, "bin", "ffplay"),
			filepath.Join(destDir, "bin", "ffplay.exe"),
		}
		for _, file := range mustNotExist {
			if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unexpected ffplay artifact extracted: %q", file)
			}
		}
	})

	t.Run("missing_required_binary_returns_error", func(t *testing.T) {
		zipPath := filepath.Join(t.TempDir(), "ffmpeg-missing-ffprobe.zip")
		createZipFixture(t, zipPath, map[string]string{
			"ffmpeg-8.0/bin/" + ffmpegBinaryName(): "ffmpeg binary",
			"ffmpeg-8.0/bin/ffplay.exe":            "ffplay binary",
		})

		err := extractFFmpeg(zipPath, t.TempDir())
		if err == nil {
			t.Fatal("extractFFmpeg() should fail when required ffprobe binary is missing")
		}
		if !strings.Contains(err.Error(), ffprobeBinaryName()) {
			t.Fatalf("error should mention missing %s, got: %v", ffprobeBinaryName(), err)
		}
	})
}

func TestGitignore_CoversFFmpegBin(t *testing.T) {
	patterns := loadGitignorePatterns(t)

	t.Run("matches_expected_paths", func(t *testing.T) {
		tests := []struct {
			name string
			path string
			want bool
		}{
			{name: "linux ffmpeg binary", path: "ffmpeg/bin/ffmpeg", want: true},
			{name: "windows ffprobe binary", path: "ffmpeg/bin/ffprobe.exe", want: true},
			{name: "general bin directory", path: "tools/bin/helper", want: true},
			{name: "regular source file", path: "audio/mixer.go", want: false},
		}

		for _, tc := range tests {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				got := matchesGitignore(patterns, tc.path)
				if got != tc.want {
					t.Fatalf("matchesGitignore(%q) = %v; want %v", tc.path, got, tc.want)
				}
			})
		}
	})

	t.Run("empty_pattern_set_does_not_match", func(t *testing.T) {
		if matchesGitignore(nil, "ffmpeg/bin/ffmpeg") {
			t.Fatal("nil .gitignore patterns should not match any path")
		}
	})
}

func createZipFixture(t *testing.T, zipPath string, files map[string]string) {
	t.Helper()

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip file: %v", err)
	}

	w := zip.NewWriter(f)
	for name, content := range files {
		entry, err := w.Create(name)
		if err != nil {
			_ = w.Close()
			_ = f.Close()
			t.Fatalf("create zip entry %q: %v", name, err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			_ = w.Close()
			_ = f.Close()
			t.Fatalf("write zip entry %q: %v", name, err)
		}
	}

	if err := w.Close(); err != nil {
		_ = f.Close()
		t.Fatalf("close zip writer: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close zip file: %v", err)
	}
}

func loadGitignorePatterns(t *testing.T) []string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	gitignorePath := filepath.Join(filepath.Dir(filepath.Dir(thisFile)), ".gitignore")

	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}

	var patterns []string
	for line := range strings.SplitSeq(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		patterns = append(patterns, trimmed)
	}
	if len(patterns) == 0 {
		t.Fatal(".gitignore should contain at least one usable pattern")
	}
	return patterns
}

func matchesGitignore(patterns []string, filePath string) bool {
	normalizedPath := path.Clean(strings.ReplaceAll(filePath, "\\", "/"))
	if normalizedPath == "." {
		return false
	}
	base := path.Base(normalizedPath)

	for _, pattern := range patterns {
		p := strings.TrimSpace(strings.ReplaceAll(pattern, "\\", "/"))
		if p == "" || strings.HasPrefix(p, "!") {
			continue
		}

		if strings.HasSuffix(p, "/") {
			dirName := strings.TrimSuffix(p, "/")
			for _, segment := range strings.Split(normalizedPath, "/") {
				if segment == dirName {
					return true
				}
			}
			continue
		}

		if matched, _ := path.Match(p, base); matched {
			return true
		}
		if matched, _ := path.Match(p, normalizedPath); matched {
			return true
		}
		if p == normalizedPath {
			return true
		}
	}

	return false
}
