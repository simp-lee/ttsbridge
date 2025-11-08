package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/simp-lee/ttsbridge/audio"
	"github.com/simp-lee/ttsbridge/providers/edgetts"
	"github.com/simp-lee/ttsbridge/tts"
)

var provider *edgetts.Provider
var cachedVoices []tts.Voice

// startCleanupTask 启动定期清理任务
func startCleanupTask() {
	ticker := time.NewTicker(1 * time.Hour) // 每小时清理一次
	defer ticker.Stop()

	// 立即执行一次清理
	cleanupUploadedFiles()

	for range ticker.C {
		cleanupUploadedFiles()
	}
}

// cleanupUploadedFiles 清理超过24小时的上传文件
func cleanupUploadedFiles() {
	tempDir := filepath.Join(os.TempDir(), "ttsbridge_music")
	if err := audio.CleanupOldFiles(tempDir, 24*time.Hour); err != nil {
		log.Printf("清理上传文件失败: %v", err)
	}
}

// checkFFmpegInstallation 检查 ffmpeg 是否已安装
func checkFFmpegInstallation() {
	cmd := exec.Command("ffmpeg", "-version")
	if err := cmd.Run(); err != nil {
		fmt.Println("⚠️  警告: ffmpeg 未安装")
		fmt.Println("   背景音乐混音功能将不可用")
		fmt.Println("   请访问 https://ffmpeg.org/download.html 下载安装")
		fmt.Println()
	} else {
		fmt.Println("✅ ffmpeg 已安装，背景音乐功能可用")
		fmt.Println()
	}
}

func main() {
	fmt.Println("🎙️  TTSBridge Web UI")
	fmt.Println("===================")
	fmt.Println()

	// 检查 ffmpeg 是否安装
	checkFFmpegInstallation()

	// 初始化提供商
	provider = edgetts.New()

	// 预加载语音列表
	fmt.Println("正在加载语音列表...")
	ctx := context.Background()
	var err error
	cachedVoices, err = provider.ListVoices(ctx, "")
	if err != nil {
		log.Fatalf("❌ 加载语音列表失败: %v", err)
	}
	fmt.Printf("✅ 已加载 %d 个语音\n\n", len(cachedVoices))

	// 启动定期清理旧的上传文件（每小时清理一次超过24小时的文件）
	go startCleanupTask()

	// 设置路由
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/api/locales", handleGetLocales)
	http.HandleFunc("/api/voices", handleGetVoices)
	http.HandleFunc("/api/synthesize", handleSynthesize)
	http.HandleFunc("/api/upload-music", handleUploadMusic)
	http.HandleFunc("/api/check-ffmpeg", handleCheckFFmpeg)
	http.HandleFunc("/api/install-ffmpeg", handleInstallFFmpeg)

	// 启动服务器
	port := "8080"
	fmt.Printf("🚀 服务器启动在: http://localhost:%s\n", port)
	fmt.Println("按 Ctrl+C 停止服务器")
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>TTSBridge - 文字转语音</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            background: #f4f6fa;
            color: #1f2933;
            min-height: 100vh;
            padding: 20px;
        }
        .container {
            max-width: 800px;
            margin: 0 auto;
            background: #ffffff;
            border-radius: 14px;
            box-shadow: 0 10px 30px rgba(31,60,136,0.08);
            overflow: hidden;
        }
        .header {
            background: linear-gradient(135deg, #1f3c88 0%, #3956b0 100%);
            color: #f8fafc;
            padding: 32px;
            text-align: center;
            border-bottom: 1px solid rgba(255,255,255,0.18);
        }
        .header h1 { font-size: 2em; margin-bottom: 10px; font-weight: 600; }
        .header p { opacity: 0.85; }
        .content { padding: 32px; }
        .form-group {
            margin-bottom: 25px;
        }
        label {
            display: block;
            margin-bottom: 8px;
            font-weight: 600;
            color: #333;
        }
        select, textarea, input {
            width: 100%;
            padding: 12px;
            border: 1px solid #d0d6e4;
            border-radius: 8px;
            font-size: 14px;
            transition: border-color 0.2s, box-shadow 0.2s;
            background: #ffffff;
        }
        select:focus, textarea:focus, input:focus {
            outline: none;
            border-color: #3956b0;
            box-shadow: 0 0 0 3px rgba(57,86,176,0.15);
        }
        textarea {
            resize: vertical;
            min-height: 120px;
            font-family: inherit;
        }
        .voice-info {
            margin-top: 10px;
            padding: 12px;
            background: #f1f4ff;
            border-radius: 10px;
            font-size: 13px;
            color: #3b4a69;
        }
        .controls {
            display: grid;
            grid-template-columns: repeat(3, 1fr);
            gap: 18px;
            margin-bottom: 28px;
        }
        .control-item label { font-size: 13px; }
        .control-item input[type="range"] {
            width: 100%;
        }
        .control-value {
            text-align: center;
            font-size: 12px;
            color: #4a5568;
            font-weight: 600;
            margin-top: 5px;
        }
        button {
            width: 100%;
            padding: 15px;
            background: #1f3c88;
            color: #f8fafc;
            border: none;
            border-radius: 8px;
            font-size: 16px;
            font-weight: 600;
            cursor: pointer;
            transition: background-color 0.25s, transform 0.1s, box-shadow 0.25s;
        }
        button:hover {
            background: #27469d;
            transform: translateY(-1px);
            box-shadow: 0 8px 20px rgba(39,70,157,0.2);
        }
        button:active { 
            transform: translateY(0);
            background: #1f3c88;
            box-shadow: none;
        }
        button:disabled {
            opacity: 0.6;
            cursor: not-allowed;
            transform: none;
        }
        #status {
            margin-top: 20px;
            padding: 15px;
            border-radius: 8px;
            text-align: center;
            display: none;
        }
    .status-loading { background: #fff6e6; color: #8a5518; display: block; }
    .status-success { background: #e3f7ef; color: #1f6f4a; display: block; }
    .status-error { background: #fdecee; color: #a23a4c; display: block; }
        #audioContainer {
            margin-top: 20px;
        }
        audio {
            width: 100%;
            border-radius: 8px;
            margin-bottom: 10px;
        }
        .download-btn {
            width: 100%;
            padding: 12px;
            background: #f1f4ff;
            color: #1f3c88;
            border: none;
            border-radius: 8px;
            font-size: 15px;
            font-weight: 600;
            cursor: pointer;
            transition: background-color 0.25s, transform 0.1s, box-shadow 0.25s;
            margin-top: 12px;
        }
        .download-btn:hover {
            background: #e3ebff;
            transform: translateY(-1px);
            box-shadow: 0 8px 20px rgba(39,70,157,0.15);
        }
        .download-btn:active { 
            transform: translateY(0);
            background: #d4dfff;
            box-shadow: none;
        }
        .loading-spinner {
            display: inline-block;
            width: 16px;
            height: 16px;
            border: 3px solid rgba(255,255,255,0.3);
            border-radius: 50%;
            border-top-color: #ffffff;
            animation: spin 1s ease-in-out infinite;
        }
        @keyframes spin {
            to { transform: rotate(360deg); }
        }
        .bg-music-section {
            background: #f8f9fc;
            padding: 20px;
            border-radius: 10px;
            margin-bottom: 25px;
        }
        .bg-music-section h3 {
            margin-bottom: 15px;
            color: #1f3c88;
            font-size: 16px;
        }
        .file-upload {
            position: relative;
            display: inline-block;
            width: 100%;
        }
        .file-upload input[type="file"] {
            display: none;
        }
        .file-upload-btn {
            display: inline-block;
            padding: 10px 20px;
            background: #3956b0;
            color: white;
            border-radius: 6px;
            cursor: pointer;
            text-align: center;
            transition: background-color 0.2s;
        }
        .file-upload-btn:hover {
            background: #27469d;
        }
        .file-name {
            margin-top: 8px;
            font-size: 13px;
            color: #4a5568;
        }
        .checkbox-group {
            display: flex;
            align-items: center;
            gap: 8px;
            margin-top: 10px;
        }
        .checkbox-group input[type="checkbox"] {
            width: auto;
        }
        
        /* FFmpeg 警告样式 */
        .ffmpeg-warning {
            background: linear-gradient(135deg, #fff5e6 0%, #fff9f0 100%);
            border: 2px solid #ffb74d;
            border-radius: 12px;
            padding: 20px;
            margin-bottom: 20px;
            display: flex;
            gap: 16px;
            align-items: flex-start;
            box-shadow: 0 4px 12px rgba(255, 152, 0, 0.15);
            animation: slideDown 0.3s ease-out;
        }
        
        @keyframes slideDown {
            from {
                opacity: 0;
                transform: translateY(-10px);
            }
            to {
                opacity: 1;
                transform: translateY(0);
            }
        }
        
        .warning-icon {
            font-size: 32px;
            line-height: 1;
            flex-shrink: 0;
        }
        
        .warning-content {
            flex: 1;
        }
        
        .warning-title {
            font-size: 16px;
            font-weight: 700;
            color: #e65100;
            margin-bottom: 8px;
        }
        
        .warning-message {
            font-size: 14px;
            color: #5d4037;
            line-height: 1.6;
            margin-bottom: 16px;
        }
        
        .warning-actions {
            display: flex;
            align-items: center;
            gap: 16px;
            flex-wrap: wrap;
        }
        
        .install-btn {
            background: linear-gradient(135deg, #ff9800 0%, #ff6f00 100%);
            color: white;
            border: none;
            padding: 12px 24px;
            border-radius: 8px;
            font-size: 15px;
            font-weight: 600;
            cursor: pointer;
            display: flex;
            align-items: center;
            gap: 8px;
            transition: all 0.3s ease;
            box-shadow: 0 4px 12px rgba(255, 152, 0, 0.3);
        }
        
        .install-btn:hover {
            background: linear-gradient(135deg, #fb8c00 0%, #f57c00 100%);
            transform: translateY(-2px);
            box-shadow: 0 6px 16px rgba(255, 152, 0, 0.4);
        }
        
        .install-btn:active {
            transform: translateY(0);
            box-shadow: 0 2px 8px rgba(255, 152, 0, 0.3);
        }
        
        .install-btn:disabled {
            background: #ccc;
            cursor: not-allowed;
            transform: none;
            box-shadow: none;
        }
        
        .btn-icon {
            font-size: 18px;
            line-height: 1;
        }
        
        .btn-text {
            line-height: 1;
        }
        
        .manual-link {
            color: #1f3c88;
            text-decoration: none;
            font-size: 14px;
            font-weight: 500;
            border-bottom: 2px solid transparent;
            transition: border-color 0.2s ease;
        }
        
        .manual-link:hover {
            border-bottom-color: #1f3c88;
        }
        
        /* 安装进度状态 */
        .install-btn.installing {
            background: linear-gradient(135deg, #78909c 0%, #546e7a 100%);
            cursor: wait;
        }
        
        .install-btn.installing .btn-icon {
            animation: rotate 1s linear infinite;
        }
        
        @keyframes rotate {
            from { transform: rotate(0deg); }
            to { transform: rotate(360deg); }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🎙️ TTSBridge</h1>
            <p>文字转语音 - 简单、快速、免费</p>
        </div>
        <div class="content">
            <form id="ttsForm">
                <div class="form-group">
                    <label for="locale">选择语言</label>
                    <select id="locale" required>
                        <option value="">加载中...</option>
                    </select>
                </div>

                <div class="form-group">
                    <label for="voice">选择语音</label>
                    <select id="voice" required>
                        <option value="">请先选择语言</option>
                    </select>
                    <div id="voiceInfo" class="voice-info" style="display:none;"></div>
                </div>

                <div class="form-group">
                    <label for="text">输入文本</label>
                    <textarea id="text" required placeholder="在这里输入要转换为语音的文本...">你好，欢迎使用 TTSBridge！这是一个简单、快速、免费的文字转语音工具。</textarea>
                </div>

                <div class="controls">
                    <div class="control-item">
                        <label for="rate">语速</label>
                        <input type="range" id="rate" min="0.5" max="2.0" step="0.1" value="1.0">
                        <div class="control-value" id="rateValue">1.0x</div>
                    </div>
                    <div class="control-item">
                        <label for="volume">音量</label>
                        <input type="range" id="volume" min="0.0" max="1.0" step="0.1" value="1.0">
                        <div class="control-value" id="volumeValue">100%</div>
                    </div>
                    <div class="control-item">
                        <label for="pitch">音调</label>
                        <input type="range" id="pitch" min="0.5" max="2.0" step="0.1" value="1.0">
                        <div class="control-value" id="pitchValue">1.0x</div>
                    </div>
                </div>

                <!-- 背景音乐配置 -->
                <div class="bg-music-section">
                    <h3>🎵 背景音乐（可选）</h3>
                    <div id="ffmpegWarning" class="ffmpeg-warning" style="display: none;">
                        <div class="warning-icon">⚠️</div>
                        <div class="warning-content">
                            <div class="warning-title">需要安装 FFmpeg</div>
                            <div class="warning-message">背景音乐混音功能依赖 FFmpeg 工具。点击下方按钮快速安装（约 100MB）。</div>
                            <div class="warning-actions">
                                <button id="installFFmpegBtn" class="install-btn">
                                    <span class="btn-icon">🚀</span>
                                    <span class="btn-text">一键安装 FFmpeg</span>
                                </button>
                                <a href="https://ffmpeg.org/download.html" target="_blank" class="manual-link">或手动下载</a>
                            </div>
                        </div>
                    </div>
                    <div class="form-group">
                        <label>上传音乐文件</label>
                        <div class="file-upload">
                            <input type="file" id="musicFile" accept=".mp3,.wav,.ogg,.flac,.m4a,.aac,.wma">
                            <label for="musicFile" class="file-upload-btn">📁 选择文件</label>
                            <div class="file-name" id="fileName">支持 MP3, WAV, OGG, FLAC 等格式</div>
                        </div>
                    </div>
                    <div class="controls">
                        <div class="control-item">
                            <label for="bgVolume">背景音乐音量</label>
                            <input type="range" id="bgVolume" min="0.0" max="1.0" step="0.1" value="0.3">
                            <div class="control-value" id="bgVolumeValue">30%</div>
                        </div>
                        <div class="control-item">
                            <label for="fadeIn">淡入时长（秒）</label>
                            <input type="range" id="fadeIn" min="0" max="5" step="0.5" value="0">
                            <div class="control-value" id="fadeInValue">0s</div>
                        </div>
                        <div class="control-item">
                            <label for="fadeOut">淡出时长（秒）</label>
                            <input type="range" id="fadeOut" min="0" max="5" step="0.5" value="0">
                            <div class="control-value" id="fadeOutValue">0s</div>
                        </div>
                    </div>
                    <div class="controls">
                        <div class="control-item">
                            <label for="startTime">起始时间（秒）</label>
                            <input type="range" id="startTime" min="0" max="30" step="1" value="0">
                            <div class="control-value" id="startTimeValue">0s</div>
                        </div>
                        <div class="control-item">
                            <label for="mainVolume">主音频音量</label>
                            <input type="range" id="mainVolume" min="0.0" max="1.0" step="0.1" value="1.0">
                            <div class="control-value" id="mainVolumeValue">100%</div>
                        </div>
                        <div class="control-item">
                            <div class="checkbox-group">
                                <input type="checkbox" id="loopMusic" checked>
                                <label for="loopMusic" style="margin: 0;">循环播放</label>
                            </div>
                        </div>
                    </div>
                </div>

                <button type="submit" id="submitBtn">🎵 生成语音</button>

                <div id="status"></div>
                <div id="audioContainer" style="display:none;">
                    <audio id="audioPlayer" controls></audio>
                    <button type="button" id="downloadBtn" class="download-btn">⬇️ 下载音频</button>
                </div>
            </form>
        </div>
    </div>

    <script>
        // 常量定义
        const ANIMATION_DELAY = 500;
        const RESET_BUTTON_DELAY = 2000;
        
        // DOM 元素引用
        const localeSelect = document.getElementById('locale');
        const voiceSelect = document.getElementById('voice');
        const voiceInfo = document.getElementById('voiceInfo');
        const statusDiv = document.getElementById('status');
        const audioPlayer = document.getElementById('audioPlayer');
        const audioContainer = document.getElementById('audioContainer');
        const submitBtn = document.getElementById('submitBtn');
        const fileNameDiv = document.getElementById('fileName');
        const musicFileInput = document.getElementById('musicFile');

        // 状态变量
        let allVoices = {};
        let uploadedMusicPath = '';
        let isUploading = false;
        let currentBlobUrl = null; // 跟踪当前的 blob URL

        // 检查 ffmpeg 是否安装
        function checkFFmpeg() {
            fetch('/api/check-ffmpeg')
                .then(res => res.json())
                .then(data => {
                    const warning = document.getElementById('ffmpegWarning');
                    warning.style.display = data.installed ? 'none' : 'block';
                })
                .catch(err => console.error('检查 ffmpeg 失败:', err));
        }
        checkFFmpeg();

        // 自动安装 ffmpeg 按钮
        document.getElementById('installFFmpegBtn').addEventListener('click', function(e) {
            e.preventDefault();
            const btn = this;
            const btnIcon = btn.querySelector('.btn-icon');
            const btnText = btn.querySelector('.btn-text');
            const originalText = btnText.textContent;
            
            // 添加安装中状态
            btn.disabled = true;
            btn.classList.add('installing');
            btnIcon.textContent = '⏳';
            btnText.textContent = '正在下载安装...';
            
            fetch('/api/install-ffmpeg', { method: 'POST' })
                .then(res => res.json())
                .then(data => {
                    if (data.success) {
                        // 成功动画
                        btnIcon.textContent = '✅';
                        btnText.textContent = '安装成功！';
                        
                        setTimeout(() => {
                            showStatus('success', '✅ ' + data.message + ' 现在可以使用背景音乐功能了！');
                            checkFFmpeg();
                        }, ANIMATION_DELAY);
                    } else {
                        btnIcon.textContent = '❌';
                        btnText.textContent = '安装失败';
                        showStatus('error', '❌ ' + (data.error || '安装失败，请手动安装'));
                    }
                })
                .catch(err => {
                    btnIcon.textContent = '❌';
                    btnText.textContent = '安装失败';
                    showStatus('error', '❌ 安装失败: ' + err.message + '\n\n请手动安装: https://ffmpeg.org/download.html');
                })
                .finally(() => {
                    setTimeout(() => {
                        btn.disabled = false;
                        btn.classList.remove('installing');
                        btnIcon.textContent = '🚀';
                        btnText.textContent = originalText;
                    }, RESET_BUTTON_DELAY);
                });
        });

        // 加载语言列表
        fetch('/api/locales')
            .then(res => res.json())
            .then(data => {
                localeSelect.innerHTML = '<option value="">请选择语言</option>';
                data.locales.forEach(locale => {
                    const option = document.createElement('option');
                    option.value = locale.code;
                    option.textContent = locale.name + ' (' + locale.count + ' 个语音)';
                    localeSelect.appendChild(option);
                });
                allVoices = data.voices;
            })
            .catch(err => {
                console.error('加载语言列表失败:', err);
                showStatus('error', '❌ 加载语言列表失败，请刷新页面重试');
            });

        // 语言改变时更新语音列表
        localeSelect.addEventListener('change', function() {
            const locale = this.value;
            voiceSelect.innerHTML = '<option value="">请选择语音</option>';
            voiceInfo.style.display = 'none';
            
            if (locale && allVoices[locale]) {
                // 按性别分组
                const groups = { '女性': [], '男性': [], '其他': [] };
                allVoices[locale].forEach(voice => {
                    if (voice.gender === 'Female') groups['女性'].push(voice);
                    else if (voice.gender === 'Male') groups['男性'].push(voice);
                    else groups['其他'].push(voice);
                });

                // 添加选项
                ['女性', '男性', '其他'].forEach(gender => {
                    if (groups[gender].length > 0) {
                        const optgroup = document.createElement('optgroup');
                        optgroup.label = gender;
                        groups[gender].forEach(voice => {
                            const option = document.createElement('option');
                            option.value = voice.short_name;
                            option.textContent = voice.display_name;
                            option.dataset.styles = JSON.stringify(voice.styles);
                            optgroup.appendChild(option);
                        });
                        voiceSelect.appendChild(optgroup);
                    }
                });
            }
        });

        // 语音改变时显示详情
        voiceSelect.addEventListener('change', function() {
            const option = this.selectedOptions[0];
            if (option && option.dataset.styles) {
                const styles = JSON.parse(option.dataset.styles);
                if (styles && styles.length > 0) {
                    voiceInfo.textContent = '风格: ' + styles.join(', ');
                    voiceInfo.style.display = 'block';
                } else {
                    voiceInfo.style.display = 'none';
                }
            } else {
                voiceInfo.style.display = 'none';
            }
        });

        // 滑块值更新（统一处理）
        const sliderFormatters = {
            'rate': (v) => v + 'x',
            'volume': (v) => Math.round(v * 100) + '%',
            'pitch': (v) => v + 'x',
            'bgVolume': (v) => Math.round(v * 100) + '%',
            'fadeIn': (v) => v + 's',
            'fadeOut': (v) => v + 's',
            'startTime': (v) => v + 's',
            'mainVolume': (v) => Math.round(v * 100) + '%'
        };
        
        Object.keys(sliderFormatters).forEach(id => {
            const slider = document.getElementById(id);
            const valueDiv = document.getElementById(id + 'Value');
            if (slider && valueDiv) {
                slider.addEventListener('input', function() {
                    valueDiv.textContent = sliderFormatters[id](parseFloat(this.value));
                });
            }
        });

        // 文件上传处理
        musicFileInput.addEventListener('change', async function(e) {
            const file = e.target.files[0];
            if (!file) {
                uploadedMusicPath = '';
                isUploading = false;
                // 重置 UI 显示为默认状态
                fileNameDiv.textContent = '支持 MP3, WAV, OGG, FLAC 等格式';
                fileNameDiv.style.color = '#4a5568';
                return;
            }
            
            isUploading = true;
            submitBtn.disabled = true;
            fileNameDiv.textContent = '上传中...';
            fileNameDiv.style.color = '#4a5568';

            const formData = new FormData();
            formData.append('music', file);

            try {
                const response = await fetch('/api/upload-music', {
                    method: 'POST',
                    body: formData
                });

                const data = await response.json();
                
                if (!response.ok) {
                    throw new Error(data.error || '上传失败');
                }

                uploadedMusicPath = data.path;
                fileNameDiv.textContent = '✅ ' + data.filename;
                fileNameDiv.style.color = '#1f6f4a';
            } catch (error) {
                let errorMsg = error.message;
                if (errorMsg.includes('Failed to fetch')) {
                    errorMsg = '网络错误，请检查服务器连接';
                }
                fileNameDiv.textContent = '❌ ' + errorMsg;
                fileNameDiv.style.color = '#a23a4c';
                uploadedMusicPath = '';
                showStatus('error', '背景音乐上传失败：' + errorMsg);
            } finally {
                isUploading = false;
                submitBtn.disabled = false;
            }
        });

        // 表单提交
        document.getElementById('ttsForm').addEventListener('submit', async function(e) {
            e.preventDefault();

            if (isUploading) {
                showStatus('error', '背景音乐正在上传中，请稍候...');
                return;
            }

            const text = document.getElementById('text').value;
            const voice = document.getElementById('voice').value;
            const rate = parseFloat(document.getElementById('rate').value);
            const volume = parseFloat(document.getElementById('volume').value);
            const pitch = parseFloat(document.getElementById('pitch').value);

            if (!voice) {
                showStatus('error', '请选择语音');
                return;
            }

            // 构建请求体
            const requestBody = { text, voice, rate, volume, pitch };

            // 如果上传了背景音乐，添加配置
            if (uploadedMusicPath) {
                requestBody.background_music = {
                    music_path: uploadedMusicPath,
                    volume: parseFloat(document.getElementById('bgVolume').value),
                    fade_in: parseFloat(document.getElementById('fadeIn').value),
                    fade_out: parseFloat(document.getElementById('fadeOut').value),
                    start_time: parseFloat(document.getElementById('startTime').value),
                    loop: document.getElementById('loopMusic').checked,
                    main_audio_volume: parseFloat(document.getElementById('mainVolume').value)
                };
            }

            submitBtn.disabled = true;
            submitBtn.innerHTML = '<span class="loading-spinner"></span> 生成中...';
            audioContainer.style.display = 'none';
            showStatus('loading', '正在生成语音，请稍候...');

            try {
                const response = await fetch('/api/synthesize', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(requestBody)
                });

                if (!response.ok) {
                    // 尝试解析 JSON 错误响应
                    const contentType = response.headers.get('content-type');
                    if (contentType && contentType.includes('application/json')) {
                        const errorData = await response.json();
                        throw new Error(errorData.error || '生成失败: ' + response.statusText);
                    } else {
                        const errorText = await response.text();
                        throw new Error(errorText || '生成失败: ' + response.statusText);
                    }
                }

                const blob = await response.blob();
                
                if (blob.size === 0) {
                    throw new Error('收到的音频数据为空');
                }
                
                // 释放之前的 blob URL 以防止内存泄漏
                if (currentBlobUrl) {
                    URL.revokeObjectURL(currentBlobUrl);
                }
                
                const url = URL.createObjectURL(blob);
                currentBlobUrl = url;
                
                // 保存音频 URL 和文件名用于下载
                audioPlayer.src = url;
                audioPlayer.dataset.audioUrl = url;
                audioPlayer.dataset.fileName = 'tts_' + voice + '_' + Date.now() + '.mp3';
                
                // 显示音频播放器和下载按钮
                audioContainer.style.display = 'block';
                showStatus('success', '✅ 生成成功！点击播放按钮试听，或下载到本地');
            } catch (error) {
                const errorMsg = error.message;
                
                // 为 ffmpeg 相关错误提供特殊提示
                if (errorMsg.includes('ffmpeg 未安装') || errorMsg.includes('背景音乐混音失败')) {
                    if (confirm('背景音乐功能需要安装 ffmpeg！\n\n是否查看安装说明？')) {
                        window.open('https://ffmpeg.org/download.html', '_blank');
                    }
                }
                
                showStatus('error', '❌ ' + errorMsg);
            } finally {
                submitBtn.disabled = false;
                submitBtn.innerHTML = '🎵 生成语音';
            }
        });

        // 下载按钮点击事件
        document.getElementById('downloadBtn').addEventListener('click', function() {
            const audioUrl = audioPlayer.dataset.audioUrl;
            const fileName = audioPlayer.dataset.fileName || 'tts_audio.mp3';
            
            if (!audioUrl) {
                showStatus('error', '❌ 没有可下载的音频');
                return;
            }

            // 创建下载链接
            const a = document.createElement('a');
            a.href = audioUrl;
            a.download = fileName;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            
            showStatus('success', '✅ 开始下载：' + fileName);
        });

        function showStatus(type, message) {
            statusDiv.className = 'status-' + type;
            statusDiv.textContent = message;
        }
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

func handleGetLocales(w http.ResponseWriter, r *http.Request) {
	// 按语言分组
	voicesByLocale := make(map[string][]tts.Voice)
	for _, voice := range cachedVoices {
		voicesByLocale[voice.Locale] = append(voicesByLocale[voice.Locale], voice)
	}

	// 获取语言列表
	var locales []string
	for locale := range voicesByLocale {
		locales = append(locales, locale)
	}
	sort.Strings(locales)

	// 常用语言排在前面
	commonLocales := []string{"zh-CN", "zh-TW", "zh-HK", "en-US", "en-GB", "ja-JP", "ko-KR"}
	var sortedLocales []string
	localeMap := make(map[string]bool)

	for _, locale := range commonLocales {
		if _, exists := voicesByLocale[locale]; exists {
			sortedLocales = append(sortedLocales, locale)
			localeMap[locale] = true
		}
	}

	for _, locale := range locales {
		if !localeMap[locale] {
			sortedLocales = append(sortedLocales, locale)
		}
	}

	// 构建响应
	type LocaleInfo struct {
		Code  string `json:"code"`
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	localeInfos := make([]LocaleInfo, 0)
	for _, locale := range sortedLocales {
		localeInfos = append(localeInfos, LocaleInfo{
			Code:  locale,
			Name:  getLocaleName(locale),
			Count: len(voicesByLocale[locale]),
		})
	}

	// 简化的语音信息（用于前端）
	type SimpleVoice struct {
		ShortName   string   `json:"short_name"`
		DisplayName string   `json:"display_name"`
		Gender      string   `json:"gender"`
		Styles      []string `json:"styles"`
	}

	simplifiedVoices := make(map[string][]SimpleVoice)
	for locale, voices := range voicesByLocale {
		simple := make([]SimpleVoice, 0)
		for _, v := range voices {
			displayName := v.DisplayName
			if displayName == "" {
				displayName = v.Name
			}
			if displayName == "" {
				displayName = v.ShortName
			}
			simple = append(simple, SimpleVoice{
				ShortName:   v.ShortName,
				DisplayName: displayName,
				Gender:      v.Gender,
				Styles:      v.Styles,
			})
		}
		simplifiedVoices[locale] = simple
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"locales": localeInfos,
		"voices":  simplifiedVoices,
	})
}

func handleGetVoices(w http.ResponseWriter, r *http.Request) {
	locale := r.URL.Query().Get("locale")

	voices := make([]tts.Voice, 0)
	for _, voice := range cachedVoices {
		if locale == "" || voice.Locale == locale {
			voices = append(voices, voice)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(voices)
}

// respondError 返回 JSON 格式的错误响应
func respondError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

// handleCheckFFmpeg 检查 ffmpeg 是否安装
func handleCheckFFmpeg(w http.ResponseWriter, r *http.Request) {
	installed := audio.IsFFmpegInstalled()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{
		"installed": installed,
	})
}

// handleInstallFFmpeg 自动安装 ffmpeg
func handleInstallFFmpeg(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "仅支持 POST 请求", http.StatusMethodNotAllowed)
		return
	}

	if audio.IsFFmpegInstalled() {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "ffmpeg 已安装",
		})
		return
	}

	if err := audio.InstallFFmpeg(); err != nil {
		respondError(w, fmt.Sprintf("安装失败: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "ffmpeg 安装成功！",
	})
}

func handleSynthesize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Text            string  `json:"text"`
		Voice           string  `json:"voice"`
		Rate            float64 `json:"rate"`
		Volume          float64 `json:"volume"`
		Pitch           float64 `json:"pitch"`
		BackgroundMusic *struct {
			MusicPath       string  `json:"music_path"`
			Volume          float64 `json:"volume"`
			FadeIn          float64 `json:"fade_in"`
			FadeOut         float64 `json:"fade_out"`
			StartTime       float64 `json:"start_time"`
			Loop            bool    `json:"loop"`
			MainAudioVolume float64 `json:"main_audio_volume"`
		} `json:"background_music"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "请求数据格式错误: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Text == "" || req.Voice == "" {
		respondError(w, "文本和语音参数不能为空", http.StatusBadRequest)
		return
	}

	opts := &tts.SynthesizeOptions{
		Text:   req.Text,
		Voice:  req.Voice,
		Rate:   req.Rate,
		Volume: req.Volume,
		Pitch:  req.Pitch,
	}

	// 如果提供了背景音乐配置，添加到选项中
	if req.BackgroundMusic != nil && req.BackgroundMusic.MusicPath != "" {
		opts.BackgroundMusic = &tts.BackgroundMusicOptions{
			MusicPath:       req.BackgroundMusic.MusicPath,
			Volume:          req.BackgroundMusic.Volume,
			FadeIn:          req.BackgroundMusic.FadeIn,
			FadeOut:         req.BackgroundMusic.FadeOut,
			StartTime:       req.BackgroundMusic.StartTime,
			Loop:            &req.BackgroundMusic.Loop,
			MainAudioVolume: req.BackgroundMusic.MainAudioVolume,
		}
	}

	ctx := context.Background()
	audio, err := provider.Synthesize(ctx, opts)
	if err != nil {
		// 提供更友好的错误提示
		errorMsg := err.Error()
		if strings.Contains(errorMsg, "ffmpeg") {
			errorMsg = "背景音乐混音失败：" + errorMsg
		} else if strings.Contains(errorMsg, "文件不存在") {
			errorMsg = "背景音乐文件未找到，请重新上传"
		}
		respondError(w, errorMsg, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Disposition", "attachment; filename=tts.mp3")
	w.Write(audio)
}

func handleUploadMusic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	// 解析 multipart form，最大 50MB
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		respondError(w, "文件过大（最大 50MB）或解析失败", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("music")
	if err != nil {
		respondError(w, "未找到上传文件，请选择文件后再试", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 读取文件内容
	fileData, err := io.ReadAll(file)
	if err != nil {
		respondError(w, "读取文件失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 先检查扩展名
	ext := filepath.Ext(header.Filename)
	if !audio.IsSupportedAudioExtension(ext) {
		respondError(w, "不支持的音频格式: "+ext+"（支持: "+audio.GetSupportedAudioExtensions()+"）", http.StatusBadRequest)
		return
	}

	// 使用 audio 包的函数保存文件
	filePath, err := audio.SaveUploadedFile(fileData, header.Filename)
	if err != nil {
		respondError(w, "保存文件失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 验证文件格式
	if err := audio.ValidateBackgroundMusicFile(filePath); err != nil {
		// 删除无效文件
		os.Remove(filePath)
		respondError(w, "音频文件验证失败: "+err.Error(), http.StatusBadRequest)
		return
	}

	// 返回文件路径
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"path":     filePath,
		"filename": header.Filename,
	})
}

func getLocaleName(locale string) string {
	names := map[string]string{
		"zh-CN": "中文-中国",
		"zh-TW": "中文-台湾",
		"zh-HK": "中文-香港",
		"en-US": "英语-美国",
		"en-GB": "英语-英国",
		"ja-JP": "日语-日本",
		"ko-KR": "韩语-韩国",
		"fr-FR": "法语-法国",
		"de-DE": "德语-德国",
		"es-ES": "西班牙语-西班牙",
		"it-IT": "意大利语-意大利",
		"ru-RU": "俄语-俄罗斯",
		"pt-BR": "葡萄牙语-巴西",
		"ar-SA": "阿拉伯语-沙特",
		"th-TH": "泰语-泰国",
		"vi-VN": "越南语-越南",
	}
	if name, exists := names[locale]; exists {
		return name
	}
	return locale
}
