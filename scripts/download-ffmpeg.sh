#!/usr/bin/env bash
#
# download-ffmpeg.sh — Download FFmpeg static builds for Windows amd64 and/or Linux amd64.
#
# Usage:
#   ./scripts/download-ffmpeg.sh                # auto-detect current platform
#   ./scripts/download-ffmpeg.sh --platform windows
#   ./scripts/download-ffmpeg.sh --platform linux
#   ./scripts/download-ffmpeg.sh --all          # download both platforms
#   ./scripts/download-ffmpeg.sh --dry-run      # resolve URL and check reachability only
#   ./scripts/download-ffmpeg.sh --skip-verify      # skip SHA-256 hash verification
#
# Binaries are placed into ffmpeg/bin/ relative to the project root.

set -euo pipefail

# ── SETUP REQUIRED ───────────────────────────────────────────────────────────
#
# SHA-256 hashes below must be set to real values. To populate them:
#   1. Run this script once:  ./scripts/download-ffmpeg.sh --all
#      The script will print the computed SHA-256 for each archive.
#   2. Copy the printed hashes and paste them into WINDOWS_SHA256 and
#      LINUX_SHA256 below, replacing the PLACEHOLDER values.
#   3. Re-run the script to confirm verification passes.
#
# IMPORTANT: With placeholders left in place, this script will fail by default
# to avoid unauthenticated binary downloads. Use --skip-verify only when
# explicitly accepting that risk.
#
# ── Configuration ────────────────────────────────────────────────────────────

# FFmpeg version to download. Update URLs and hashes below when changing version.
FFMPEG_VERSION="8.0.1"

WINDOWS_URL="https://www.gyan.dev/ffmpeg/builds/packages/ffmpeg-${FFMPEG_VERSION}-essentials_build.zip"
LINUX_RELEASES_API_BASE="${LINUX_RELEASES_API_BASE:-https://api.github.com/repos/BtbN/FFmpeg-Builds/releases}"
LINUX_RELEASES_PER_PAGE="${LINUX_RELEASES_PER_PAGE:-100}"
LINUX_RELEASES_PAGES="${LINUX_RELEASES_PAGES:-3}"

# Expected SHA-256 hashes for the pinned version archives.
# These must be real values; placeholders cause a hard failure unless
# --skip-verify is explicitly provided.
WINDOWS_SHA256="PLACEHOLDER_UPDATE_AFTER_FIRST_DOWNLOAD"
LINUX_SHA256="PLACEHOLDER_UPDATE_AFTER_FIRST_DOWNLOAD"

# ── Globals ──────────────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
BIN_DIR="${PROJECT_ROOT}/ffmpeg/bin"
TMP_DIR=""
SKIP_VERIFY=false
DRY_RUN=false

# ── Helpers ──────────────────────────────────────────────────────────────────

info()  { printf "\033[1;34m[INFO]\033[0m  %s\n" "$*"; }
warn()  { printf "\033[1;33m[WARN]\033[0m  %s\n" "$*"; }
error() { printf "\033[1;31m[ERROR]\033[0m %s\n" "$*" >&2; }
die()   { error "$@"; exit 1; }

cleanup() {
    if [[ -n "${TMP_DIR}" && -d "${TMP_DIR}" ]]; then
        info "Cleaning up temporary files..."
        rm -rf "${TMP_DIR}"
    fi
}
trap cleanup EXIT

# Create a temporary directory for downloads.
make_tmp() {
    TMP_DIR="$(mktemp -d)"
    info "Using temp directory: ${TMP_DIR}"
}

# Download a file using curl or wget (whichever is available).
download() {
    local url="$1" dest="$2"
    info "Downloading: ${url}"
    if command -v curl &>/dev/null; then
        curl -fSL --progress-bar -o "${dest}" "${url}"
    elif command -v wget &>/dev/null; then
        wget -q --show-progress -O "${dest}" "${url}"
    else
        die "Neither curl nor wget found. Please install one of them."
    fi
}

# Check whether URL is reachable (follow redirects and avoid HEAD-only false results).
check_url_reachable() {
    local url="$1"
    info "Checking URL reachability: ${url}"
    if command -v curl &>/dev/null; then
        if curl -fILSs "${url}" >/dev/null; then
            return 0
        fi
        warn "HEAD probe failed; retrying with ranged GET probe."
        if curl -fLSs --range 0-0 -o /dev/null "${url}"; then
            return 0
        fi
        warn "Ranged GET probe failed; retrying with full GET probe."
        curl -fLSs -o /dev/null "${url}"
    elif command -v wget &>/dev/null; then
        if wget -q --max-redirect=10 --spider "${url}"; then
            return 0
        fi
        warn "Wget spider probe failed; retrying with GET probe."
        wget -q --max-redirect=10 -O /dev/null "${url}"
    else
        die "Neither curl nor wget found. Please install one of them."
    fi
}

# Build releases API URL with per_page/page parameters.
build_releases_api_url() {
    local page="$1"
    local sep="?"
    if [[ "${LINUX_RELEASES_API_BASE}" == *\?* ]]; then
        sep="&"
    fi
    echo "${LINUX_RELEASES_API_BASE}${sep}per_page=${LINUX_RELEASES_PER_PAGE}&page=${page}"
}

# Discover Linux release asset URLs from GitHub Releases API.
fetch_linux_release_urls() {
    local page url payload parsed_urls
    for page in $(seq 1 "${LINUX_RELEASES_PAGES}"); do
        url="$(build_releases_api_url "${page}")"
        payload=""
        parsed_urls=""

        if command -v curl &>/dev/null; then
            payload="$(curl -fsSL \
                -H "Accept: application/vnd.github+json" \
                -H "User-Agent: ttsbridge-download-script" \
                "${url}" || true)"
        elif command -v wget &>/dev/null; then
            payload="$(wget -qO- \
                --header="Accept: application/vnd.github+json" \
                --header="User-Agent: ttsbridge-download-script" \
                "${url}" || true)"
        else
            die "Neither curl nor wget found. Please install one of them."
        fi

        if [[ -z "${payload}" ]]; then
            warn "Failed to fetch Linux release metadata (api=${url}, page=${page})." >&2
            continue
        fi

        if command -v jq &>/dev/null; then
            parsed_urls="$(printf '%s\n' "${payload}" \
                | jq -r '((.[]?.assets[]?.browser_download_url?), (.[]?.browser_download_url?), (.assets[]?.browser_download_url?), (.browser_download_url?)) // empty' 2>/dev/null \
                | grep '/releases/download/' || true)"

            if [[ -n "${parsed_urls}" ]]; then
                info "Parsed Linux release metadata with jq (api=${url}, page=${page})." >&2
                printf '%s\n' "${parsed_urls}"
                continue
            fi

            warn "jq parser returned no release URLs; falling back to grep/sed (api=${url}, page=${page})." >&2
        fi

        parsed_urls="$(printf '%s\n' "${payload}" \
            | grep -oE 'https://[^"[:space:]]+' \
            | sed 's#\\/#/#g' \
            | grep '/releases/download/' || true)"

        if [[ -n "${parsed_urls}" ]]; then
            printf '%s\n' "${parsed_urls}"
            continue
        fi

        warn "Failed to parse Linux release URLs from metadata (api=${url}, page=${page})." >&2
    done
}

# Resolve a Linux download URL from release metadata.
resolve_linux_url() {
    if [[ -n "${LINUX_FFMPEG_URL:-}" ]]; then
        info "Using Linux URL from LINUX_FFMPEG_URL override." >&2
        echo "${LINUX_FFMPEG_URL}"
        return
    fi

    local major_minor version_re escaped_version escaped_track
    major_minor="${FFMPEG_VERSION%.*}"
    version_re="${FFMPEG_VERSION//./\\.}"
    escaped_version="${version_re}"
    escaped_track="${major_minor//./\\.}"

    local exact_pattern track_pattern release_urls resolved
    exact_pattern="ffmpeg-n${escaped_version}(-[^/]+)*-linux64-gpl-${escaped_track}\\.tar\\.xz$"
    track_pattern="ffmpeg-n${escaped_track}(\\.[0-9]+)?(-[^/]+)*-linux64-gpl-${escaped_track}\\.tar\\.xz$"
    release_urls="$(fetch_linux_release_urls)"
    resolved=""

    if [[ -n "${release_urls}" ]]; then
        if [[ -n "${LINUX_FFMPEG_TAG:-}" ]]; then
            resolved="$(printf '%s\n' "${release_urls}" | grep "/releases/download/${LINUX_FFMPEG_TAG}/" | grep -E "${exact_pattern}" | head -1 || true)"
            if [[ -n "${resolved}" ]]; then
                info "Resolved Linux URL via releases API (tag=${LINUX_FFMPEG_TAG}, exact version match)." >&2
                echo "${resolved}"
                return
            fi

            resolved="$(printf '%s\n' "${release_urls}" | grep "/releases/download/${LINUX_FFMPEG_TAG}/" | grep -E "${track_pattern}" | head -1 || true)"
            if [[ -n "${resolved}" ]]; then
                warn "Exact Linux asset not found under tag ${LINUX_FFMPEG_TAG}; using track fallback under same tag." >&2
                echo "${resolved}"
                return
            fi
        fi

        resolved="$(printf '%s\n' "${release_urls}" | grep -E "${exact_pattern}" | head -1 || true)"
        if [[ -n "${resolved}" ]]; then
            info "Resolved Linux URL via releases API (exact version match)." >&2
            echo "${resolved}"
            return
        fi

        resolved="$(printf '%s\n' "${release_urls}" | grep -E "${track_pattern}" | head -1 || true)"
        if [[ -n "${resolved}" ]]; then
            warn "Exact Linux asset not found; using release API track fallback." >&2
            echo "${resolved}"
            return
        fi
    else
        warn "Could not read Linux release metadata from GitHub API." >&2
    fi

    die "Unable to resolve Linux FFmpeg archive for version ${FFMPEG_VERSION}. Set LINUX_FFMPEG_URL to an exact asset URL, or increase LINUX_RELEASES_PAGES / set LINUX_FFMPEG_TAG."
}

# Compute SHA-256 hash of a file.
sha256_of() {
    local file="$1"
    if command -v sha256sum &>/dev/null; then
        sha256sum "${file}" | awk '{print $1}'
    elif command -v shasum &>/dev/null; then
        shasum -a 256 "${file}" | awk '{print $1}'
    else
        warn "No sha256sum or shasum found; cannot compute hash."
        echo ""
    fi
}

# Verify SHA-256 hash of a downloaded archive against the built-in expected hash.
# Verification runs by default; use --skip-verify to bypass.
verify_hash() {
    local file="$1" label="$2" expected="$3"
    if [[ "${SKIP_VERIFY}" == true ]]; then
        warn "Hash verification skipped (--skip-verify)."
        return
    fi
    local actual
    actual="$(sha256_of "${file}")"
    if [[ -z "${actual}" ]]; then
        die "Hash verification is required, but no sha256 tool is available. Install sha256sum or shasum, or use --skip-verify explicitly."
    fi
    info "SHA-256 (${label}): ${actual}"
    if [[ "${expected}" == PLACEHOLDER_* ]]; then
        die "Hash verification is required, but ${label} hash is still a placeholder. Set a real SHA-256 or use --skip-verify explicitly."
    fi
    if [[ "${actual}" == "${expected}" ]]; then
        info "Hash verification PASSED."
    else
        die "Hash verification FAILED. Expected: ${expected}, Got: ${actual}"
    fi
}

# Verify that an extracted ffmpeg binary reports the expected version.
# This guards against the Linux "latest" floating tag delivering a mismatched build.
verify_binary_version() {
    local binary="$1"
    # Only run if the binary is executable on this OS (skip Windows .exe on Linux).
    if [[ ! -x "${binary}" ]] || [[ "${binary}" == *.exe ]]; then
        return
    fi
    local version_output
    version_output="$("${binary}" -version 2>&1 | head -1)" || true
    if echo "${version_output}" | grep -q "${FFMPEG_VERSION}"; then
        info "Version check PASSED: binary reports ${FFMPEG_VERSION}."
    else
        warn "Version check FAILED: expected ${FFMPEG_VERSION} in output, got:"
        warn "  ${version_output}"
        die "Downloaded ffmpeg binary does not match expected version ${FFMPEG_VERSION}. The upstream URL may have changed."
    fi
}

# ── Platform downloaders ─────────────────────────────────────────────────────

download_windows() {
    info "=== Downloading FFmpeg for Windows amd64 ==="
    if [[ "${DRY_RUN}" == true ]]; then
        check_url_reachable "${WINDOWS_URL}"
        info "Dry-run mode: skipping Windows download/extract."
        return
    fi

    make_tmp
    local archive="${TMP_DIR}/ffmpeg-windows.zip"
    download "${WINDOWS_URL}" "${archive}"
    verify_hash "${archive}" "windows-zip" "${WINDOWS_SHA256}"

    info "Extracting Windows binaries..."
    # The zip contains a top-level directory like ffmpeg-N.N-essentials_build/bin/
    # We need ffmpeg.exe and ffprobe.exe from it.
    if ! command -v unzip &>/dev/null; then
        die "'unzip' is required to extract the Windows archive."
    fi

    # List matching files, extract to tmp, then copy.
    local extract_dir="${TMP_DIR}/win_extract"
    mkdir -p "${extract_dir}"
    unzip -q -o "${archive}" -d "${extract_dir}"

    # Find the binaries inside the extracted tree.
    local ffmpeg_exe ffprobe_exe
    ffmpeg_exe="$(find "${extract_dir}" -type f -name 'ffmpeg.exe' | head -1)"
    ffprobe_exe="$(find "${extract_dir}" -type f -name 'ffprobe.exe' | head -1)"

    if [[ -z "${ffmpeg_exe}" ]]; then
        die "ffmpeg.exe not found in the downloaded archive."
    fi
    if [[ -z "${ffprobe_exe}" ]]; then
        die "ffprobe.exe not found in the downloaded archive."
    fi

    mkdir -p "${BIN_DIR}"
    cp -f "${ffmpeg_exe}"  "${BIN_DIR}/ffmpeg.exe"
    cp -f "${ffprobe_exe}" "${BIN_DIR}/ffprobe.exe"

    info "Windows binaries installed:"
    info "  ${BIN_DIR}/ffmpeg.exe"
    info "  ${BIN_DIR}/ffprobe.exe"
}

download_linux() {
    info "=== Downloading FFmpeg for Linux amd64 ==="
    local linux_url
    linux_url="$(resolve_linux_url)"

    if [[ "${DRY_RUN}" == true ]]; then
        check_url_reachable "${linux_url}"
        info "Dry-run mode: skipping Linux download/extract."
        return
    fi

    make_tmp
    local archive="${TMP_DIR}/ffmpeg-linux.tar.xz"
    download "${linux_url}" "${archive}"
    verify_hash "${archive}" "linux-tar.xz" "${LINUX_SHA256}"

    info "Extracting Linux binaries..."
    if ! command -v tar &>/dev/null; then
        die "'tar' is required to extract the Linux archive."
    fi

    local extract_dir="${TMP_DIR}/linux_extract"
    mkdir -p "${extract_dir}"
    tar -xf "${archive}" -C "${extract_dir}"

    # Find the binaries inside the extracted tree.
    local ffmpeg_bin ffprobe_bin
    ffmpeg_bin="$(find "${extract_dir}" -type f -name 'ffmpeg' ! -name '*.exe' | head -1)"
    ffprobe_bin="$(find "${extract_dir}" -type f -name 'ffprobe' ! -name '*.exe' | head -1)"

    if [[ -z "${ffmpeg_bin}" ]]; then
        die "ffmpeg binary not found in the downloaded archive."
    fi
    if [[ -z "${ffprobe_bin}" ]]; then
        die "ffprobe binary not found in the downloaded archive."
    fi

    mkdir -p "${BIN_DIR}"
    cp -f "${ffmpeg_bin}"  "${BIN_DIR}/ffmpeg"
    cp -f "${ffprobe_bin}" "${BIN_DIR}/ffprobe"

    chmod +x "${BIN_DIR}/ffmpeg"
    chmod +x "${BIN_DIR}/ffprobe"

    # Verify the extracted binary reports the expected version.
    verify_binary_version "${BIN_DIR}/ffmpeg"

    info "Linux binaries installed:"
    info "  ${BIN_DIR}/ffmpeg"
    info "  ${BIN_DIR}/ffprobe"
}

# ── Detect current platform ──────────────────────────────────────────────────

detect_platform() {
    local os arch
    os="$(uname -s)"
    arch="$(uname -m)"

    # Only amd64 (x86_64) binaries are provided.
    case "${arch}" in
        x86_64|amd64) ;; # supported
        *) die "Unsupported architecture: ${arch}. This script only provides amd64 (x86_64) binaries." ;;
    esac

    case "${os}" in
        Linux*)   echo "linux"   ;;
        MINGW*|MSYS*|CYGWIN*|Windows_NT) echo "windows" ;;
        *)        die "Unsupported OS: ${os}. Use --platform to specify manually." ;;
    esac
}

# ── Main ─────────────────────────────────────────────────────────────────────

usage() {
    cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Download FFmpeg static builds into ffmpeg/bin/.

Options:
    --platform <windows|linux>  Download for the specified platform only.
    --all                       Download for both Windows and Linux.
    --dry-run                   Resolve URLs and verify they are reachable only.
    --skip-verify               Skip SHA-256 hash verification (not recommended).
    -h, --help                  Show this help message.

If no option is given, the current platform is auto-detected.

Examples:
    $(basename "$0")                      # auto-detect
    $(basename "$0") --platform linux
    $(basename "$0") --all
    $(basename "$0") --platform windows --skip-verify
EOF
}

main() {
    local platform=""
    local download_all=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --platform)
                [[ $# -ge 2 ]] || die "--platform requires an argument (windows|linux)."
                platform="$2"
                shift 2
                ;;
            --all)
                download_all=true
                shift
                ;;
            --skip-verify)
                SKIP_VERIFY=true
                shift
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                die "Unknown option: $1. Use --help for usage."
                ;;
        esac
    done

    info "Project root: ${PROJECT_ROOT}"
    info "Target directory: ${BIN_DIR}"

    if [[ "${download_all}" == true ]]; then
        download_windows
        # Reset TMP_DIR so cleanup re-creates for the next download.
        cleanup
        TMP_DIR=""
        download_linux
    elif [[ -n "${platform}" ]]; then
        case "${platform}" in
            windows) download_windows ;;
            linux)   download_linux   ;;
            *)       die "Unknown platform '${platform}'. Use 'windows' or 'linux'." ;;
        esac
    else
        local detected
        detected="$(detect_platform)"
        info "Auto-detected platform: ${detected}"
        case "${detected}" in
            windows) download_windows ;;
            linux)   download_linux   ;;
        esac
    fi

    info "Done."
}

main "$@"
