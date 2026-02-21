package audio

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	ffmpegWindowsVersion = "8.0.1"

	// Windows 64位 ffmpeg Essentials 版本（约 100MB，包含常用编解码器，推荐）
	ffmpegOfficialURL = "https://www.gyan.dev/ffmpeg/builds/packages/ffmpeg-" + ffmpegWindowsVersion + "-essentials_build.zip"
	// 备用镜像（GitHub Release GPL 完整版，约 197MB，包含所有编解码器）
	ffmpegGitHubURL = "https://github.com/BtbN/FFmpeg-Builds/releases/download/autobuild-2026-02-20-13-01/ffmpeg-n8.0.1-win64-gpl-8.0.zip"

	// 默认哈希保留占位，要求通过环境变量显式提供真实值后才允许下载。
	// 这样可确保在无法验证完整性时默认失败（secure-by-default）。
	ffmpegOfficialSHA256 = "PLACEHOLDER_UPDATE_WITH_REAL_SHA256"
	ffmpegGitHubSHA256   = "PLACEHOLDER_UPDATE_WITH_REAL_SHA256"
)

var ffmpegDownloadTimeout = 10 * time.Minute

type ffmpegDownloadSource struct {
	name    string
	url     string
	hash    string
	hashEnv string
}

// ffmpegBinaryName 返回当前平台的 ffmpeg 二进制文件名
func ffmpegBinaryName() string {
	if runtime.GOOS == "windows" {
		return "ffmpeg.exe"
	}
	return "ffmpeg"
}

// ffprobeBinaryName 返回当前平台的 ffprobe 二进制文件名
func ffprobeBinaryName() string {
	if runtime.GOOS == "windows" {
		return "ffprobe.exe"
	}
	return "ffprobe"
}

// getLibraryFFmpegDir 基于库自身源码路径定位 ffmpeg/bin/ 目录
// 使用 runtime.Caller 获取此文件的绝对路径，然后向上导航到库根目录的 ffmpeg/bin/
func getLibraryFFmpegDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	// thisFile = .../ttsbridge/audio/ffmpeg_installer.go
	// 向上一级到库根目录，再进入 ffmpeg/bin/
	libRoot := filepath.Dir(filepath.Dir(thisFile))
	return filepath.Join(libRoot, "ffmpeg", "bin")
}

// findBinary 在多个位置查找指定的二进制文件
// 查找顺序：系统 PATH → 可执行文件目录/bin/ → 库目录 ffmpeg/bin/
// 返回找到的完整路径和是否找到
func findBinary(name string) (string, bool) {
	// 1. 检查系统 PATH
	if path, err := exec.LookPath(name); err == nil {
		return path, true
	}

	// 2. 检查可执行文件目录下的本地安装目录
	if installDir, err := getInstallDir(); err == nil {
		binDir := filepath.Join(installDir, "bin")
		fullPath := filepath.Join(binDir, name)
		if _, err := os.Stat(fullPath); err == nil {
			addToPath(binDir)
			return fullPath, true
		}
	}

	// 3. 检查库自身的 ffmpeg/bin/ 目录
	if libDir := getLibraryFFmpegDir(); libDir != "" {
		fullPath := filepath.Join(libDir, name)
		if _, err := os.Stat(fullPath); err == nil {
			addToPath(libDir)
			return fullPath, true
		}
	}

	return "", false
}

// IsFFmpegInstalled 检查 ffmpeg 是否已安装
// 查找顺序：系统 PATH → 可执行文件目录/bin/ → 库目录 ffmpeg/bin/
func IsFFmpegInstalled() bool {
	_, found := findBinary(ffmpegBinaryName())
	return found
}

// IsFFprobeInstalled 检查 ffprobe 是否已安装
// 查找顺序：系统 PATH → 可执行文件目录/bin/ → 库目录 ffmpeg/bin/
func IsFFprobeInstalled() bool {
	_, found := findBinary(ffprobeBinaryName())
	return found
}

// getInstallDir 获取安装目录（程序所在目录下的 ffmpeg 文件夹）
func getInstallDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("获取程序路径失败: %w", err)
	}
	return filepath.Join(filepath.Dir(exePath), "ffmpeg"), nil
}

// InstallFFmpeg 自动下载并安装 ffmpeg（仅支持 Windows）。
//
// 推荐的安装方式（按优先级）：
//  1. 系统包管理器安装（apt/brew/choco），确保 ffmpeg 在 PATH 中
//  2. 使用 scripts/download-ffmpeg.sh 下载到项目 ffmpeg/bin/ 目录
//  3. 本函数作为 Windows-only 的后备方案
//
// Deprecated: 推荐使用 scripts/download-ffmpeg.sh 或系统包管理器安装。
func InstallFFmpeg() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("自动安装仅支持 Windows 系统")
	}

	sources, err := ffmpegWindowsDownloadSources()
	if err != nil {
		return err
	}

	installDir, err := getInstallDir()
	if err != nil {
		return err
	}

	// 创建安装目录
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("创建安装目录失败: %w", err)
	}

	// 检查是否已有本地安装
	ffmpegExe := filepath.Join(installDir, "bin", ffmpegBinaryName())
	if _, err := os.Stat(ffmpegExe); err == nil {
		// 已存在，添加到 PATH
		return addToPath(filepath.Join(installDir, "bin"))
	}

	// 下载 ffmpeg（带备用源）
	zipPath := filepath.Join(installDir, "ffmpeg.zip")
	needDownload := false

	// 检查是否已经下载过 zip 文件
	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		needDownload = true
	} else {
		// 验证 zip 文件是否完整且哈希可信
		if err := validateZipFile(zipPath); err != nil {
			fmt.Printf("检测到损坏的 zip 文件，重新下载...\n")
			os.Remove(zipPath)
			needDownload = true
		} else if err := verifyArchiveFromTrustedSources(zipPath, sources); err != nil {
			fmt.Printf("检测到未通过完整性校验的 zip 文件，重新下载...\n")
			os.Remove(zipPath)
			needDownload = true
		}
	}

	if needDownload {
		fmt.Println("正在下载 ffmpeg...")

		tmpZipPath := zipPath + ".downloading"
		_ = os.Remove(tmpZipPath)

		var downloadErrors []string
		downloaded := false
		for _, source := range sources {
			fmt.Printf("下载源: %s (%s)\n", source.name, source.url)
			if err := downloadFile(tmpZipPath, source.url); err != nil {
				downloadErrors = append(downloadErrors, fmt.Sprintf("%s 下载失败: %v", source.name, err))
				_ = os.Remove(tmpZipPath)
				continue
			}

			if err := verifyFileSHA256(tmpZipPath, source.hash); err != nil {
				downloadErrors = append(downloadErrors, fmt.Sprintf("%s 完整性校验失败: %v", source.name, err))
				_ = os.Remove(tmpZipPath)
				continue
			}

			if err := os.Rename(tmpZipPath, zipPath); err != nil {
				_ = os.Remove(tmpZipPath)
				return fmt.Errorf("落地下载文件失败: %w", err)
			}

			downloaded = true
			break
		}

		if !downloaded {
			return fmt.Errorf("所有下载源均失败或未通过校验: %s\n请手动下载: %s", strings.Join(downloadErrors, " | "), "https://ffmpeg.org/download.html")
		}
		fmt.Println("✓ 下载完成")
	}

	fmt.Println("正在解压 ffmpeg...")
	if err := extractFFmpeg(zipPath, installDir); err != nil {
		return fmt.Errorf("解压失败: %w", err)
	}

	// 添加到 PATH（临时，仅当前程序）
	if err := addToPath(filepath.Join(installDir, "bin")); err != nil {
		return fmt.Errorf("添加到 PATH 失败: %w", err)
	}

	fmt.Println("✓ ffmpeg 安装成功!")
	fmt.Printf("安装位置: %s\n", ffmpegExe)
	return nil
}

func ffmpegWindowsDownloadSources() ([]ffmpegDownloadSource, error) {
	sourceDefs := []ffmpegDownloadSource{
		{name: "gyan.dev Essentials 版本", url: ffmpegOfficialURL, hash: ffmpegOfficialSHA256, hashEnv: "TTSBRIDGE_FFMPEG_OFFICIAL_SHA256"},
		{name: "GitHub Release 备用源", url: ffmpegGitHubURL, hash: ffmpegGitHubSHA256, hashEnv: "TTSBRIDGE_FFMPEG_GITHUB_SHA256"},
	}

	sources := make([]ffmpegDownloadSource, 0, len(sourceDefs))
	var errs []string
	for _, def := range sourceDefs {
		expectedHash, err := resolveExpectedHash(def.hashEnv, def.hash)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", def.name, err))
			continue
		}

		def.hash = expectedHash
		sources = append(sources, def)
	}

	if len(sources) == 0 {
		return nil, fmt.Errorf("未配置可校验的 ffmpeg 下载源哈希，请设置 TTSBRIDGE_FFMPEG_OFFICIAL_SHA256 或 TTSBRIDGE_FFMPEG_GITHUB_SHA256: %s", strings.Join(errs, " | "))
	}

	return sources, nil
}

func resolveExpectedHash(envName, fallback string) (string, error) {
	h := strings.TrimSpace(os.Getenv(envName))
	if h == "" {
		h = strings.TrimSpace(fallback)
	}
	if h == "" || strings.HasPrefix(h, "PLACEHOLDER_") {
		return "", fmt.Errorf("%s 未配置真实 SHA-256", envName)
	}

	normalized, err := normalizeSHA256(h)
	if err != nil {
		return "", fmt.Errorf("%s 值不合法: %w", envName, err)
	}

	return normalized, nil
}

func normalizeSHA256(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) != 64 {
		return "", fmt.Errorf("SHA-256 长度应为 64，实际为 %d", len(trimmed))
	}

	decoded, err := hex.DecodeString(trimmed)
	if err != nil {
		return "", fmt.Errorf("SHA-256 不是有效十六进制: %w", err)
	}

	return hex.EncodeToString(decoded), nil
}

func fileSHA256(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func verifyFileSHA256(filePath, expected string) error {
	normalizedExpected, err := normalizeSHA256(expected)
	if err != nil {
		return err
	}

	actual, err := fileSHA256(filePath)
	if err != nil {
		return err
	}

	if actual != normalizedExpected {
		return fmt.Errorf("SHA-256 不匹配: expected=%s actual=%s", normalizedExpected, actual)
	}

	return nil
}

func verifyArchiveFromTrustedSources(zipPath string, sources []ffmpegDownloadSource) error {
	var mismatchErrors []string
	for _, source := range sources {
		if err := verifyFileSHA256(zipPath, source.hash); err == nil {
			return nil
		} else {
			mismatchErrors = append(mismatchErrors, source.name)
		}
	}

	return fmt.Errorf("文件与所有受信任下载源哈希均不匹配: %s", strings.Join(mismatchErrors, ", "))
}

// validateZipFile 验证 zip 文件是否完整可用
func validateZipFile(zipPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	// 检查是否至少有一些文件
	if len(r.File) == 0 {
		return fmt.Errorf("zip 文件为空")
	}

	return nil
}

// downloadFile 下载文件，带进度显示
func downloadFile(filepath string, url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), ffmpegDownloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("创建下载请求失败 (%s): %w", url, err)
	}

	client := &http.Client{Timeout: ffmpegDownloadTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载请求失败 (%s): %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败 (%s): HTTP %d", url, resp.StatusCode)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// 显示下载进度
	buf := make([]byte, 32*1024)
	var downloaded int64
	total := resp.ContentLength

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			written, writeErr := out.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}
			downloaded += int64(n)
			if total > 0 {
				fmt.Printf("\r下载进度: %.0f%% (%.1f/%.1f MB)",
					float64(downloaded)/float64(total)*100,
					float64(downloaded)/1024/1024,
					float64(total)/1024/1024)
			}
		}
		if err == io.EOF {
			fmt.Println()
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// extractFFmpeg 解压 ffmpeg 压缩包
func extractFFmpeg(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("打开 zip 文件失败: %w", err)
	}
	defer r.Close()

	binDir := filepath.Join(destDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("创建 bin 目录失败: %w", err)
	}

	// 统计成功提取的文件
	extractedCount := 0
	ffmpegName := ffmpegBinaryName()
	ffprobeName := ffprobeBinaryName()
	targetFiles := map[string]bool{
		ffmpegName:  false,
		ffprobeName: false,
	}

	// 只提取需要的二进制文件
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}

		name := filepath.Base(f.Name)
		if name == ffmpegName || name == ffprobeName {
			destPath := filepath.Join(binDir, name)
			if err := extractFile(f, destPath); err != nil {
				return fmt.Errorf("解压 %s 失败: %w", name, err)
			}
			fmt.Printf("✓ 已解压: %s\n", name)
			targetFiles[name] = true
			extractedCount++
		}
	}

	// 检查是否至少提取了 ffmpeg 和 ffprobe
	if !targetFiles[ffmpegName] {
		return fmt.Errorf("zip 文件中未找到 %s，可能文件已损坏，请删除 %s 后重试", ffmpegName, zipPath)
	}
	if !targetFiles[ffprobeName] {
		return fmt.Errorf("zip 文件中未找到 %s，可能文件已损坏，请删除 %s 后重试", ffprobeName, zipPath)
	}

	if extractedCount == 0 {
		return fmt.Errorf("未能从 zip 文件中提取任何文件，可能文件已损坏，请删除 %s 后重试", zipPath)
	}

	return nil
}

// extractFile 解压单个文件
func extractFile(f *zip.File, destPath string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}

// addToPath 添加目录到 PATH 环境变量（仅当前进程）
func addToPath(dir string) error {
	currentPath := os.Getenv("PATH")
	cleanDir := filepath.Clean(dir)
	for _, entry := range filepath.SplitList(currentPath) {
		if entry == "" {
			continue
		}
		cleanEntry := filepath.Clean(entry)
		if runtime.GOOS == "windows" {
			if strings.EqualFold(cleanEntry, cleanDir) {
				return nil // 已存在
			}
			continue
		}
		if cleanEntry == cleanDir {
			return nil // 已存在
		}
	}

	newPath := dir + string(os.PathListSeparator) + currentPath
	return os.Setenv("PATH", newPath)
}

// CleanupFFmpegZip 清理已下载的 ffmpeg zip 文件（节省空间）。
//
// Deprecated: 推荐使用 scripts/download-ffmpeg.sh 或系统包管理器安装。
func CleanupFFmpegZip() error {
	installDir, err := getInstallDir()
	if err != nil {
		return err
	}

	zipPath := filepath.Join(installDir, "ffmpeg.zip")
	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		return nil // 文件不存在，无需清理
	}

	if err := os.Remove(zipPath); err != nil {
		return fmt.Errorf("删除 zip 文件失败: %w", err)
	}

	fmt.Printf("已清理 ffmpeg.zip 文件: %s\n", zipPath)
	return nil
}
