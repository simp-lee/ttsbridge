package audio

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	// Windows 64位 ffmpeg Essentials 版本（约 100MB，包含常用编解码器，推荐）
	ffmpegOfficialURL = "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"
	// 备用镜像（GitHub Release GPL 完整版，约 197MB，包含所有编解码器）
	ffmpegGitHubURL = "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-gpl.zip"
)

// IsFFmpegInstalled 检查 ffmpeg 是否已安装（系统 PATH 或本地目录）
func IsFFmpegInstalled() bool {
	// 先检查系统 PATH
	cmd := exec.Command("ffmpeg", "-version")
	if cmd.Run() == nil {
		return true
	}

	// 检查本地安装目录
	installDir, err := getInstallDir()
	if err != nil {
		return false
	}

	ffmpegExe := filepath.Join(installDir, "bin", "ffmpeg.exe")
	if _, err := os.Stat(ffmpegExe); err == nil {
		// 本地存在，添加到当前进程的 PATH
		addToPath(filepath.Join(installDir, "bin"))
		return true
	}

	return false
}

// getInstallDir 获取安装目录（程序所在目录下的 ffmpeg 文件夹）
func getInstallDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("获取程序路径失败: %w", err)
	}
	return filepath.Join(filepath.Dir(exePath), "ffmpeg"), nil
}

// InstallFFmpeg 自动下载并安装 ffmpeg
func InstallFFmpeg() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("自动安装仅支持 Windows 系统")
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
	ffmpegExe := filepath.Join(installDir, "bin", "ffmpeg.exe")
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
		// 验证 zip 文件是否完整
		if err := validateZipFile(zipPath); err != nil {
			fmt.Printf("检测到损坏的 zip 文件，重新下载...\n")
			os.Remove(zipPath)
			needDownload = true
		}
	}

	if needDownload {
		fmt.Println("正在下载 ffmpeg...")

		// 优先使用官方构建版本（体积更小）
		fmt.Println("下载源: gyan.dev Essentials 版本 (约 100MB，请稍候...)")
		err = downloadFile(zipPath, ffmpegOfficialURL)
		if err != nil {
			fmt.Printf("官方源下载失败: %v\n", err)
			fmt.Println("尝试备用源: GitHub...")

			// 尝试备用源
			err = downloadFile(zipPath, ffmpegGitHubURL)
			if err != nil {
				return fmt.Errorf("所有下载源均失败: %w\n请手动下载: %s", err, "https://ffmpeg.org/download.html")
			}
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
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
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
			out.Write(buf[:n])
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
	targetFiles := map[string]bool{
		"ffmpeg.exe":  false,
		"ffprobe.exe": false,
		"ffplay.exe":  false,
	}

	// 只提取需要的 .exe 文件
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}

		name := filepath.Base(f.Name)
		if name == "ffmpeg.exe" || name == "ffprobe.exe" || name == "ffplay.exe" {
			destPath := filepath.Join(binDir, name)
			if err := extractFile(f, destPath); err != nil {
				return fmt.Errorf("解压 %s 失败: %w", name, err)
			}
			fmt.Printf("✓ 已解压: %s\n", name)
			targetFiles[name] = true
			extractedCount++
		}
	}

	// 检查是否至少提取了 ffmpeg.exe 和 ffprobe.exe
	if !targetFiles["ffmpeg.exe"] {
		return fmt.Errorf("zip 文件中未找到 ffmpeg.exe，可能文件已损坏，请删除 %s 后重试", zipPath)
	}
	if !targetFiles["ffprobe.exe"] {
		return fmt.Errorf("zip 文件中未找到 ffprobe.exe，可能文件已损坏，请删除 %s 后重试", zipPath)
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
	if strings.Contains(currentPath, dir) {
		return nil // 已存在
	}

	newPath := dir + string(os.PathListSeparator) + currentPath
	return os.Setenv("PATH", newPath)
}

// CleanupFFmpegZip 清理已下载的 ffmpeg zip 文件（节省空间）
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
