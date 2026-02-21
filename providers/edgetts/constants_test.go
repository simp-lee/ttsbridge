package edgetts

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode"
)

func TestChromiumVersion_DocumentedAndConsistent(t *testing.T) {
	if chromiumFullVersion == "" {
		t.Fatal("chromiumFullVersion should not be empty")
	}

	wantMajor, err := chromiumMajorFromFullVersion(chromiumFullVersion)
	if err != nil {
		t.Fatalf("invalid chromiumFullVersion %q: %v", chromiumFullVersion, err)
	}
	if chromiumMajorVersion != wantMajor {
		t.Fatalf("chromiumMajorVersion = %q; want %q from chromiumFullVersion", chromiumMajorVersion, wantMajor)
	}

	wantSecMsGecVersion := "1-" + chromiumFullVersion
	if secMsGecVersion != wantSecMsGecVersion {
		t.Fatalf("secMsGecVersion = %q; want %q", secMsGecVersion, wantSecMsGecVersion)
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	constantsPath := filepath.Join(filepath.Dir(thisFile), "constants.go")
	content, err := os.ReadFile(constantsPath)
	if err != nil {
		t.Fatalf("read constants.go: %v", err)
	}

	text := string(content)
	requiredDocFragments := []string{
		"最后同步:",
		"CHROMIUM_FULL_VERSION",
		chromiumFullVersion,
	}
	for _, fragment := range requiredDocFragments {
		if !strings.Contains(text, fragment) {
			t.Fatalf("constants.go should document Chromium version, missing fragment: %q", fragment)
		}
	}
}

func TestChromiumMajorFromFullVersion_ErrorPaths(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
		wantErr bool
	}{
		{name: "valid semantic-like version", version: "143.0.3650.75", want: "143"},
		{name: "valid simple major.minor", version: "99.1", want: "99"},
		{name: "empty version", version: "", wantErr: true},
		{name: "missing major", version: ".0.3650.75", wantErr: true},
		{name: "non-digit major", version: "v143.0.1", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := chromiumMajorFromFullVersion(tc.version)
			if (err != nil) != tc.wantErr {
				t.Fatalf("chromiumMajorFromFullVersion(%q) error = %v; wantErr=%v", tc.version, err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if got != tc.want {
				t.Fatalf("chromiumMajorFromFullVersion(%q) = %q; want %q", tc.version, got, tc.want)
			}
		})
	}
}

func chromiumMajorFromFullVersion(version string) (string, error) {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" {
		return "", fmt.Errorf("empty version")
	}

	major := strings.SplitN(trimmed, ".", 2)[0]
	if major == "" {
		return "", fmt.Errorf("missing major version")
	}
	for _, r := range major {
		if !unicode.IsDigit(r) {
			return "", fmt.Errorf("major version %q contains non-digit character %q", major, r)
		}
	}
	return major, nil
}
