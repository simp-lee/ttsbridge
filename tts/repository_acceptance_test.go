package tts_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

type goSourceFile struct {
	RelPath  string
	AST      *ast.File
	Package  string
	Imported []string
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}

	return filepath.Dir(filepath.Dir(file))
}

func readRepositoryFile(t *testing.T, root, rel string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}

	return string(data)
}

func walkRepositoryTextFiles(t *testing.T, root string, relPaths []string, visit func(rel string, content string)) {
	t.Helper()

	textExt := map[string]bool{
		".go":   true,
		".md":   true,
		".mod":  true,
		".sum":  true,
		".txt":  true,
		".yaml": true,
		".yml":  true,
		".json": true,
	}

	for _, relPath := range relPaths {
		fullPath := filepath.Join(root, filepath.FromSlash(relPath))
		info, err := os.Stat(fullPath)
		if err != nil {
			t.Fatalf("stat %s: %v", relPath, err)
		}

		if !info.IsDir() {
			visit(filepath.ToSlash(relPath), readRepositoryFile(t, root, relPath))
			continue
		}

		err = filepath.WalkDir(fullPath, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !textExt[strings.ToLower(filepath.Ext(d.Name()))] {
				return nil
			}

			walkRelPath, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}

			visit(filepath.ToSlash(walkRelPath), readRepositoryFile(t, root, filepath.ToSlash(walkRelPath)))
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", relPath, err)
		}
	}
}

func walkRepositoryFiles(t *testing.T, root string, visit func(rel string)) {
	t.Helper()

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		visit(filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk repository files: %v", err)
	}
}

func isNonTestTextFile(rel string) bool {
	return !strings.HasSuffix(rel, "_test.go")
}

func requireMissingPath(t *testing.T, root, rel string) {
	t.Helper()

	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
		t.Fatalf("expected %s to be absent", rel)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", rel, err)
	}
}

func requireDirectory(t *testing.T, root, rel string) {
	t.Helper()

	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("stat %s: %v", rel, err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", rel)
	}
}

type exportedSymbol struct {
	PackageRel string
	Kind       string
	Name       string
}

func collectExportedSymbols(t *testing.T, root, relDir string) []exportedSymbol {
	t.Helper()

	symbols := collectExportedPackageSymbols(t, root, relDir)
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].PackageRel != symbols[j].PackageRel {
			return symbols[i].PackageRel < symbols[j].PackageRel
		}
		if symbols[i].Kind != symbols[j].Kind {
			return symbols[i].Kind < symbols[j].Kind
		}
		return symbols[i].Name < symbols[j].Name
	})

	return symbols
}

func collectExportedPackageSymbols(t *testing.T, root, relDir string) []exportedSymbol {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(relDir)))
	if err != nil {
		t.Fatalf("read package directory %s: %v", relDir, err)
	}

	fset := token.NewFileSet()
	symbols := make([]exportedSymbol, 0, len(entries))

	for _, entry := range entries {
		if !isNonTestGoDirEntry(entry) {
			continue
		}

		file := parseGoFileOrFatal(t, fset, root, filepath.ToSlash(filepath.Join(relDir, entry.Name())), parser.SkipObjectResolution)
		symbols = append(symbols, collectExportedSymbolsFromFile(relDir, file)...)
	}

	return symbols
}

func isNonTestGoDirEntry(entry fs.DirEntry) bool {
	return !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go")
}

func parseGoFileOrFatal(t *testing.T, fset *token.FileSet, root, relPath string, mode parser.Mode) *ast.File {
	t.Helper()

	filePath := filepath.Join(root, filepath.FromSlash(relPath))
	file, err := parser.ParseFile(fset, filePath, nil, mode)
	if err != nil {
		t.Fatalf("parse %s: %v", relPath, err)
	}
	return file
}

func collectExportedSymbolsFromFile(relDir string, file *ast.File) []exportedSymbol {
	symbols := make([]exportedSymbol, 0, len(file.Decls))
	for _, decl := range file.Decls {
		symbols = append(symbols, collectExportedSymbolsFromDecl(relDir, decl)...)
	}
	return symbols
}

func collectExportedSymbolsFromDecl(relDir string, decl ast.Decl) []exportedSymbol {
	switch d := decl.(type) {
	case *ast.GenDecl:
		return collectExportedSymbolsFromGenDecl(relDir, d)
	case *ast.FuncDecl:
		return collectExportedSymbolsFromFuncDecl(relDir, d)
	default:
		return nil
	}
}

func collectExportedSymbolsFromGenDecl(relDir string, decl *ast.GenDecl) []exportedSymbol {
	switch decl.Tok {
	case token.TYPE:
		var symbols []exportedSymbol
		for _, spec := range decl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			symbols = append(symbols, collectExportedSymbolsFromTypeSpec(relDir, typeSpec)...)
		}
		return symbols
	case token.CONST, token.VAR:
		var symbols []exportedSymbol
		for _, spec := range decl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			symbols = append(symbols, collectExportedSymbolsFromValueSpec(relDir, decl.Tok, valueSpec)...)
		}
		return symbols
	default:
		return nil
	}
}

func collectExportedSymbolsFromTypeSpec(relDir string, typeSpec *ast.TypeSpec) []exportedSymbol {
	if !ast.IsExported(typeSpec.Name.Name) {
		return nil
	}

	typeName := typeSpec.Name.Name
	symbols := []exportedSymbol{{PackageRel: relDir, Kind: "type", Name: typeName}}
	return append(symbols, collectExportedMemberSymbols(relDir, typeName, typeSpec.Type)...)
}

func collectExportedMemberSymbols(relDir, typeName string, expr ast.Expr) []exportedSymbol {
	switch node := expr.(type) {
	case *ast.StructType:
		return collectExportedStructFieldSymbols(relDir, typeName, node.Fields)
	case *ast.InterfaceType:
		return collectExportedInterfaceMethodSymbols(relDir, typeName, node.Methods)
	default:
		return nil
	}
}

func collectExportedStructFieldSymbols(relDir, typeName string, fields *ast.FieldList) []exportedSymbol {
	if fields == nil {
		return nil
	}

	var symbols []exportedSymbol
	for _, field := range fields.List {
		for _, name := range field.Names {
			if ast.IsExported(name.Name) {
				symbols = append(symbols, exportedSymbol{PackageRel: relDir, Kind: "field", Name: typeName + "." + name.Name})
			}
		}
	}
	return symbols
}

func collectExportedInterfaceMethodSymbols(relDir, typeName string, methods *ast.FieldList) []exportedSymbol {
	if methods == nil {
		return nil
	}

	var symbols []exportedSymbol
	for _, field := range methods.List {
		for _, name := range field.Names {
			symbols = append(symbols, exportedSymbol{PackageRel: relDir, Kind: "method", Name: typeName + "." + name.Name})
		}
	}
	return symbols
}

func collectExportedSymbolsFromValueSpec(relDir string, tok token.Token, spec *ast.ValueSpec) []exportedSymbol {
	var symbols []exportedSymbol
	for _, name := range spec.Names {
		if ast.IsExported(name.Name) {
			symbols = append(symbols, exportedSymbol{PackageRel: relDir, Kind: strings.ToLower(tok.String()), Name: name.Name})
		}
	}
	return symbols
}

func collectExportedSymbolsFromFuncDecl(relDir string, decl *ast.FuncDecl) []exportedSymbol {
	if decl.Recv == nil {
		if ast.IsExported(decl.Name.Name) {
			return []exportedSymbol{{PackageRel: relDir, Kind: "func", Name: decl.Name.Name}}
		}
		return nil
	}

	receiver := receiverTypeName(decl.Recv.List[0].Type)
	if receiver == "" || !ast.IsExported(receiver) || !ast.IsExported(decl.Name.Name) {
		return nil
	}

	return []exportedSymbol{{PackageRel: relDir, Kind: "method", Name: receiver + "." + decl.Name.Name}}
}

func collectExportedTopLevelTypeNames(t *testing.T, root, relDir string) []string {
	t.Helper()

	symbols := collectExportedSymbols(t, root, relDir)
	seen := make(map[string]struct{})
	var names []string
	for _, symbol := range symbols {
		if symbol.Kind != "type" {
			continue
		}
		if _, ok := seen[symbol.Name]; ok {
			continue
		}
		seen[symbol.Name] = struct{}{}
		names = append(names, symbol.Name)
	}
	sort.Strings(names)
	return names
}

func collectNonTestGoSourceFiles(t *testing.T, root, relDir string) []goSourceFile {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(relDir)))
	if err != nil {
		t.Fatalf("read package directory %s: %v", relDir, err)
	}

	fset := token.NewFileSet()
	files := make([]goSourceFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		relPath := filepath.ToSlash(filepath.Join(relDir, entry.Name()))
		filePath := filepath.Join(root, filepath.FromSlash(relPath))
		file, err := parser.ParseFile(fset, filePath, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", relPath, err)
		}

		imported := make([]string, 0, len(file.Imports))
		for _, spec := range file.Imports {
			imported = append(imported, strings.Trim(spec.Path.Value, "\""))
		}

		files = append(files, goSourceFile{
			RelPath:  relPath,
			AST:      file,
			Package:  file.Name.Name,
			Imported: imported,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].RelPath < files[j].RelPath
	})

	return files
}

func collectPublicLibraryPackages(t *testing.T, root string) []string {
	t.Helper()

	packages := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		return collectPublicLibraryPackageEntry(root, path, d, walkErr, packages)
	})
	if err != nil {
		t.Fatalf("walk public packages: %v", err)
	}

	paths := make([]string, 0, len(packages))
	for dir, pkgName := range packages {
		if base := filepath.Base(dir); base != pkgName {
			t.Fatalf("public package path %s should align with package name %s", dir, pkgName)
		}
		paths = append(paths, dir)
	}
	sort.Strings(paths)
	return paths
}

func collectPublicLibraryPackageEntry(root, path string, d fs.DirEntry, walkErr error, packages map[string]string) error {
	if walkErr != nil {
		return walkErr
	}
	if d.IsDir() {
		return publicLibraryDirDecision(root, path, d.Name())
	}
	if !isNonTestGoSourceFile(d.Name()) {
		return nil
	}

	rel, err := repositoryRelativePath(root, path)
	if err != nil {
		return err
	}
	dir := filepath.ToSlash(filepath.Dir(rel))
	if dir == "." {
		return nil
	}

	pkgName, err := parsePackageName(path)
	if err != nil {
		return err
	}
	if pkgName == "main" {
		return nil
	}

	packages[dir] = pkgName
	return nil
}

func publicLibraryDirDecision(root, path, name string) error {
	if strings.HasPrefix(name, ".") {
		return filepath.SkipDir
	}

	rel, err := repositoryRelativePath(root, path)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	if isNonLibraryDirectory(rel) {
		return filepath.SkipDir
	}
	return nil
}

func repositoryRelativePath(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func isNonLibraryDirectory(rel string) bool {
	return strings.HasPrefix(rel, "cmd/") || strings.HasPrefix(rel, "examples/") || strings.HasPrefix(rel, "internal/")
}

func isNonTestGoSourceFile(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
}

func parsePackageName(path string) (string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.PackageClauseOnly)
	if err != nil {
		return "", err
	}
	return file.Name.Name, nil
}

func receiverTypeName(expr ast.Expr) string {
	switch node := expr.(type) {
	case *ast.Ident:
		return node.Name
	case *ast.StarExpr:
		return receiverTypeName(node.X)
	case *ast.IndexExpr:
		return receiverTypeName(node.X)
	case *ast.IndexListExpr:
		return receiverTypeName(node.X)
	default:
		return ""
	}
}

func lowerContainsAny(value string, fragments []string) bool {
	lower := strings.ToLower(value)
	for _, fragment := range fragments {
		if strings.Contains(lower, strings.ToLower(fragment)) {
			return true
		}
	}
	return false
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func collectDuplicateExportedTypeNamesByPackage(t *testing.T, root string, relDirs []string) map[string][]string {
	t.Helper()

	typeOwners := make(map[string][]string)
	for _, relDir := range relDirs {
		for _, typeName := range collectExportedTopLevelTypeNames(t, root, relDir) {
			typeOwners[typeName] = append(typeOwners[typeName], relDir)
		}
	}

	duplicates := make(map[string][]string)
	for typeName, owners := range typeOwners {
		if len(owners) < 2 {
			continue
		}
		sort.Strings(owners)
		duplicates[typeName] = owners
	}

	return duplicates
}

// AC-2: 当前公开的本地音频处理层应从核心库中删除，不再作为对外支持能力保留。
func TestRepositoryBoundary_NoPublicAudioLayer(t *testing.T) {
	root := repositoryRoot(t)
	requireMissingPath(t, root, "audio")

	forbiddenRefs := []string{
		"github.com/simp-lee/ttsbridge/audio",
		"simp-lee/ttsbridge/audio",
	}

	walkRepositoryTextFiles(t, root, []string{"README.md", "cmd", "examples", "providers", "tts"}, func(rel string, content string) {
		if !isNonTestTextFile(rel) {
			return
		}
		for _, forbidden := range forbiddenRefs {
			if strings.Contains(content, forbidden) {
				t.Fatalf("found forbidden audio layer reference %q in %s", forbidden, rel)
			}
		}
	})
}

// AC-4: 核心库不得在任何公共路径中依赖 ffmpeg 或 ffprobe。
func TestPublicPackages_DoNotReferenceFFmpegTools(t *testing.T) {
	root := repositoryRoot(t)
	allowedManualCheckFile := "examples/audio_quality/README.md"
	manualCheckDisclaimer := "仅用于本地手工检查输出文件信息，不是 TTS Bridge 库或 CLI 的运行时依赖。"

	walkRepositoryTextFiles(t, root, []string{"README.md", "cmd", "examples", "providers", "tts"}, func(rel string, content string) {
		if !isNonTestTextFile(rel) {
			return
		}
		lower := strings.ToLower(content)
		mentionsFFmpeg := strings.Contains(lower, "ffmpeg")
		mentionsFFprobe := strings.Contains(lower, "ffprobe")
		if !mentionsFFmpeg && !mentionsFFprobe {
			return
		}

		if rel == allowedManualCheckFile {
			if mentionsFFmpeg {
				t.Fatalf("manual-check example should not mention ffmpeg in %s", rel)
			}
			if !strings.Contains(content, manualCheckDisclaimer) {
				t.Fatalf("manual-check disclaimer missing from %s", rel)
			}
			return
		}

		if mentionsFFmpeg || mentionsFFprobe {
			t.Fatalf("found forbidden ffmpeg/ffprobe reference in %s", rel)
		}
	})
}

// AC-5: WebUI 应从主仓库目标交付中删除。
func TestRepositoryBoundary_WebUICommandAbsent(t *testing.T) {
	root := repositoryRoot(t)
	requireMissingPath(t, root, "cmd/webui")

	for _, rel := range []string{"README.md", "cmd/ttsbridge/README.md"} {
		lower := strings.ToLower(readRepositoryFile(t, root, rel))
		if strings.Contains(lower, "webui") || strings.Contains(lower, "web ui") {
			t.Fatalf("unexpected WebUI reference in %s", rel)
		}
	}
}

// AC-6: 对外定位必须明确为 Go 包用于程序内集成，CLI 用于命令行与脚本自动化。
func TestREADME_StatesLibraryAndCLIOnly(t *testing.T) {
	root := repositoryRoot(t)
	readme := readRepositoryFile(t, root, "README.md")
	cliReadme := readRepositoryFile(t, root, "cmd/ttsbridge/README.md")

	if !strings.Contains(readme, "Go 语言通用文字转语音 (TTS) 库") {
		t.Fatal("root README no longer states the library positioning")
	}
	if !strings.Contains(readme, "go get github.com/simp-lee/ttsbridge") {
		t.Fatal("root README no longer documents Go package consumption")
	}
	if !strings.Contains(readme, "CLI") {
		t.Fatal("root README no longer documents the CLI consumption path")
	}
	if !strings.Contains(cliReadme, "命令行工具入口") {
		t.Fatal("CLI README no longer states the command-line positioning")
	}
	if !strings.Contains(cliReadme, "go run ./cmd/ttsbridge --help") {
		t.Fatal("CLI README no longer documents direct CLI execution")
	}
	if !strings.Contains(cliReadme, "go build -o ttsbridge ./cmd/ttsbridge") {
		t.Fatal("CLI README no longer documents building the CLI binary")
	}
	for _, required := range []string{"SynthesisRequest", "SynthesisResult", "ProviderCapabilities", "VoiceFilter"} {
		if !strings.Contains(readme, required) {
			t.Fatalf("root README must document unified contract type %q", required)
		}
	}
	for _, forbidden := range []string{"Provider[T]", "edgetts.SynthesizeOptions", "volcengine.SynthesizeOptions"} {
		if strings.Contains(readme, forbidden) {
			t.Fatalf("root README must not advertise legacy/provider-specific API fragment %q", forbidden)
		}
	}
}

// AC-7: README、示例和文档必须删去背景音乐混音与 WebUI 的官方能力表述。
func TestDocs_DoNotAdvertiseBackgroundMusicOrWebUI(t *testing.T) {
	root := repositoryRoot(t)
	forbiddenPhrases := []string{
		"背景音乐",
		"backgroundmusic",
		"background_music",
		"webui",
		"web ui",
	}

	for _, rel := range []string{"README.md", "cmd/ttsbridge/README.md", "examples/audio_quality/README.md"} {
		lower := strings.ToLower(readRepositoryFile(t, root, rel))
		for _, forbidden := range forbiddenPhrases {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("found forbidden documentation phrase %q in %s", forbidden, rel)
			}
		}
	}
}

// AC-8: 测试结构必须同步收敛，禁止本地媒体处理与 WebUI 相关测试重新进入仓库。
func TestRepositoryTests_DoNotReintroduceRemovedAudioOrWebUITestSuites(t *testing.T) {
	root := repositoryRoot(t)
	allowedRoots := []string{
		"internal/cli/",
		"providers/edgetts/",
		"providers/volcengine/",
		"tts/",
	}
	forbiddenPathFragments := []string{
		"/audio/",
		"/webui/",
		"cmd/webui/",
		"examples/background_music/",
	}
	seenTests := 0

	walkRepositoryFiles(t, root, func(rel string) {
		if !strings.HasSuffix(rel, "_test.go") {
			return
		}

		seenTests++

		for _, forbidden := range forbiddenPathFragments {
			if strings.Contains("/"+rel, forbidden) {
				t.Fatalf("found forbidden removed-feature test path %s", rel)
			}
		}

		for _, allowedRoot := range allowedRoots {
			if strings.HasPrefix(rel, allowedRoot) {
				return
			}
		}

		t.Fatalf("test file %s is outside the TTS-focused test roots", rel)
	})

	if seenTests == 0 {
		t.Fatal("expected repository to contain TTS-focused test suites")
	}
}

// AC-15: 文档与目录结构必须能直接体现 Library + CLI 的最终形态。
func TestRepositoryStructure_ReflectsLibraryPlusCLI(t *testing.T) {
	root := repositoryRoot(t)

	requireDirectory(t, root, "tts")
	requireDirectory(t, root, "providers")
	requireDirectory(t, root, "examples")
	requireDirectory(t, root, "cmd/ttsbridge")
	requireMissingPath(t, root, "audio")
	requireMissingPath(t, root, "cmd/webui")
	requireMissingPath(t, root, "examples/background_music")

	entries, err := os.ReadDir(filepath.Join(root, "cmd"))
	if err != nil {
		t.Fatalf("read cmd directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "ttsbridge" || !entries[0].IsDir() {
		t.Fatalf("cmd directory = %v; want only ttsbridge/", entries)
	}

	exampleEntries, err := os.ReadDir(filepath.Join(root, "examples"))
	if err != nil {
		t.Fatalf("read examples directory: %v", err)
	}
	if len(exampleEntries) == 0 {
		t.Fatal("examples directory should retain TTS-focused examples")
	}
	for _, entry := range exampleEntries {
		if entry.Name() == "background_music" {
			t.Fatal("examples/background_music should be absent")
		}
	}
}

// AC-9: 设计必须优先满足名称一致、边界纯净、概念最小化。
// AC-13: 核心库 API 设计必须保持简洁、准确、无冗余。
func TestPublicPackageInventory_RemainsMinimalAndPurposeNamed(t *testing.T) {
	root := repositoryRoot(t)
	got := collectPublicLibraryPackages(t, root)
	want := []string{
		"providers/edgetts",
		"providers/volcengine",
		"tts",
		"tts/textutils",
	}

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("public package inventory = %v, want %v", got, want)
	}
}

// AC-10: CLI 二进制应作为正式辅助消费方式保留，但其定位应从属于核心库能力。
func TestCLIEntryPoint_RemainsThinAndDelegatesToInternalCLI(t *testing.T) {
	root := repositoryRoot(t)
	files := collectNonTestGoSourceFiles(t, root, "cmd/ttsbridge")
	if len(files) != 1 {
		t.Fatalf("cmd/ttsbridge non-test Go files = %d, want 1 thin entrypoint", len(files))
	}

	mainFile := files[0]
	if mainFile.RelPath != "cmd/ttsbridge/main.go" {
		t.Fatalf("CLI entrypoint = %s, want cmd/ttsbridge/main.go", mainFile.RelPath)
	}

	var repoImports []string
	for _, imported := range mainFile.Imported {
		if strings.Contains(imported, "github.com/simp-lee/ttsbridge/") {
			repoImports = append(repoImports, imported)
		}
		if strings.Contains(imported, "/providers/") || strings.HasSuffix(imported, "/tts") {
			t.Fatalf("CLI entrypoint must not depend directly on library/provider packages: %s", imported)
		}
	}

	wantImports := []string{"github.com/simp-lee/ttsbridge/internal/cli"}
	if strings.Join(repoImports, ",") != strings.Join(wantImports, ",") {
		t.Fatalf("CLI entrypoint repo imports = %v, want %v", repoImports, wantImports)
	}
}

// AC-10: CLI 二进制应作为正式辅助消费方式保留，但其定位应从属于核心库能力。
func TestRepositoryDocs_KeepCLIOperationalDetailsSecondaryToLibraryDocs(t *testing.T) {
	root := repositoryRoot(t)
	readme := readRepositoryFile(t, root, "README.md")
	cliReadme := readRepositoryFile(t, root, "cmd/ttsbridge/README.md")

	libraryPosition := strings.Index(readme, "Go 语言通用文字转语音 (TTS) 库")
	cliMention := strings.Index(readme, "CLI")
	if libraryPosition < 0 || cliMention < 0 {
		t.Fatalf("root README should mention both library positioning and CLI path")
	}
	if libraryPosition > cliMention {
		t.Fatalf("root README mentions CLI before the library-first positioning")
	}

	for _, forbidden := range []string{"退出码（Exit Code）", "--stdout", "--max-attempts"} {
		if strings.Contains(readme, forbidden) {
			t.Fatalf("root README should keep CLI operational detail %q in the dedicated CLI docs", forbidden)
		}
	}

	for _, required := range []string{"ttsbridge synthesize", "ttsbridge voices", "go run ./cmd/ttsbridge --help", "退出码（Exit Code）", "--stdout", "--max-attempts"} {
		if !strings.Contains(cliReadme, required) {
			t.Fatalf("CLI README must retain operational detail %q", required)
		}
	}
}

// AC-11: CLI 应复用核心库能力，而不是复制另一套 provider 逻辑或配置模型。
func TestCLIProviderAwareness_StaysConfinedToAdapterFiles(t *testing.T) {
	root := repositoryRoot(t)
	files := collectNonTestGoSourceFiles(t, root, "internal/cli")
	for _, file := range files {
		for _, imported := range file.Imported {
			if !strings.HasPrefix(imported, "github.com/simp-lee/ttsbridge/providers/") {
				continue
			}
			if !strings.HasPrefix(filepath.Base(file.RelPath), "provider_") {
				t.Fatalf("provider import %s must stay in provider adapter files, found in %s", imported, file.RelPath)
			}
		}
	}
}

// AC-11: CLI 应直接复用统一 tts 契约，而不是复制另一套 request/result/capability 模型。
func TestCLIContracts_ReuseUnifiedTTSContracts(t *testing.T) {
	root := repositoryRoot(t)
	internalCLITypes := collectExportedTopLevelTypeNames(t, root, "internal/cli")

	for _, required := range []string{"ProviderConfig", "ProviderFactory", "ProviderRegistration"} {
		if !stringSliceContains(internalCLITypes, required) {
			t.Fatalf("expected internal/cli to retain %s for CLI-only provider wiring", required)
		}
	}
	for _, forbidden := range []string{"SynthesizeRequest", "SynthesisResult", "ProviderCapabilities", "Voice", "VoiceFilter"} {
		if stringSliceContains(internalCLITypes, forbidden) {
			t.Fatalf("internal/cli must not shadow unified core contract type %s", forbidden)
		}
	}

	registrySource := readRepositoryFile(t, root, "internal/cli/registry.go")
	if !strings.Contains(registrySource, "type ProviderFactory func(cfg *ProviderConfig) (tts.Provider, error)") {
		t.Fatal("CLI registry must create unified tts.Provider instances")
	}
	if !strings.Contains(registrySource, "type ProviderRegistration struct") {
		t.Fatal("CLI registry must carry provider registration metadata instead of a bespoke adapter interface")
	}
	if strings.Contains(registrySource, "ListVoices(ctx context.Context, locale string)") {
		t.Fatal("legacy locale-only adapter signature must not remain in CLI registry")
	}

	voicesSource := readRepositoryFile(t, root, "internal/cli/voices.go")
	if !strings.Contains(voicesSource, "tts.VoiceFilter{Language: c.locale}") {
		t.Fatal("voices command must translate CLI locale input into unified tts.VoiceFilter")
	}

	synthesizeSource := readRepositoryFile(t, root, "internal/cli/synthesize.go")
	for _, required := range []string{"tts.SynthesisRequest{", "tts.ProsodyParams{"} {
		if !strings.Contains(synthesizeSource, required) {
			t.Fatalf("synthesize command must reuse unified core contract fragment %q", required)
		}
	}

	walkRepositoryTextFiles(t, root, []string{"internal/cli"}, func(rel string, content string) {
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return
		}
		for _, forbidden := range []string{
			"type Voice struct",
			"type VoiceExtra struct",
			"type SynthesizeRequest struct",
			"type SynthesisResult struct",
			"type ProviderCapabilities struct",
			"type EdgeTTS",
			"type Volcengine",
		} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("CLI layer should not shadow provider/core data models; found %q in %s", forbidden, rel)
			}
		}
	})
}

var publicAPIPackages = []string{
	"tts",
	"tts/textutils",
	"providers/edgetts",
	"providers/volcengine",
}

var forbiddenBoundaryFragments = []string{
	"backgroundmusic",
	"background_music",
	"mixer",
	"mix",
	"upload",
	"webui",
	"ffmpeg",
	"ffprobe",
	"cleanupoldfiles",
	"supportedaudioextension",
}

var allowedCoreTypeDomains = map[string][]string{
	"tts": {
		"Language",
		"Gender",
		"Boundary",
		"Voice",
		"Synthesis",
		"Prosody",
		"Audio",
		"Provider",
		"Error",
		"Format",
		"Output",
		"Health",
		"Failure",
		"Fallback",
	},
	"tts/textutils": {
		"Split",
		"Clean",
	},
}

// AC-13: 核心库 API 设计必须保持简洁、准确、无冗余，避免为未确认需求预留多余抽象、包装层或配置层。
func TestPublicAPI_PublicExportsAvoidForbiddenBoundaryConcepts(t *testing.T) {
	root := repositoryRoot(t)
	for _, relDir := range publicAPIPackages {
		relDir := relDir
		t.Run(strings.ReplaceAll(relDir, "/", "_"), func(t *testing.T) {
			assertPublicExportsAvoidForbiddenFragments(t, root, relDir, forbiddenBoundaryFragments)
		})
	}
}

func TestPublicAPI_ExportedTypesStayWithinApprovedDomains(t *testing.T) {
	root := repositoryRoot(t)
	for relDir, allowedFragments := range allowedCoreTypeDomains {
		relDir := relDir
		allowedFragments := allowedFragments
		t.Run(strings.ReplaceAll(relDir, "/", "_"), func(t *testing.T) {
			assertPublicTypesStayWithinDomains(t, root, relDir, allowedFragments)
		})
	}
}

func assertPublicExportsAvoidForbiddenFragments(t *testing.T, root, relDir string, forbiddenFragments []string) {
	t.Helper()

	for _, symbol := range collectExportedSymbols(t, root, relDir) {
		if lowerContainsAny(symbol.Name, forbiddenFragments) {
			t.Fatalf("public %s %s in %s leaks removed boundary concepts", symbol.Kind, symbol.Name, relDir)
		}
	}
}

func assertPublicTypesStayWithinDomains(t *testing.T, root, relDir string, allowedFragments []string) {
	t.Helper()

	for _, typeName := range collectExportedTopLevelTypeNames(t, root, relDir) {
		if !lowerContainsAny(typeName, allowedFragments) {
			t.Fatalf("exported type %s in %s falls outside the approved TTS domains %v", typeName, relDir, allowedFragments)
		}
	}
}

func TestPublicAPI_CLIRequestAndConfigContractsRemainInternal(t *testing.T) {
	root := repositoryRoot(t)
	internalCLITypes := collectExportedTopLevelTypeNames(t, root, "internal/cli")
	for _, internalName := range []string{"ProviderConfig", "ProviderFactory", "ProviderRegistration"} {
		if !stringSliceContains(internalCLITypes, internalName) {
			t.Fatalf("expected internal/cli to define %s for CLI-only adaptation", internalName)
		}

		for _, relDir := range publicAPIPackages {
			publicTypes := collectExportedTopLevelTypeNames(t, root, relDir)
			if stringSliceContains(publicTypes, internalName) {
				t.Fatalf("public package %s must not expose CLI-only contract type %s", relDir, internalName)
			}
		}
	}
}

func TestPublicAPI_UnifiedProviderContractReplacesGenericAPI(t *testing.T) {
	root := repositoryRoot(t)
	typesSource := readRepositoryFile(t, root, "tts/types.go")
	if !strings.Contains(typesSource, "type Provider interface") {
		t.Fatal("tts/types.go must expose a unified Provider interface")
	}
	for _, required := range []string{"type SynthesisRequest struct", "type SynthesisResult struct", "type ProviderCapabilities struct"} {
		if !strings.Contains(typesSource, required) {
			t.Fatalf("tts/types.go must define unified contract fragment %q", required)
		}
	}
	for _, forbidden := range []string{"type Provider[T any]", "Provider[T]"} {
		if strings.Contains(typesSource, forbidden) {
			t.Fatalf("tts/types.go must not retain legacy generic provider fragment %q", forbidden)
		}
	}
}

func TestPublicAPI_ProviderPackagesDoNotExportLegacySynthesizeOptions(t *testing.T) {
	root := repositoryRoot(t)
	for _, relDir := range []string{"providers/edgetts", "providers/volcengine"} {
		publicTypes := collectExportedTopLevelTypeNames(t, root, relDir)
		if stringSliceContains(publicTypes, "SynthesizeOptions") {
			t.Fatalf("public package %s must not export legacy provider-specific SynthesizeOptions", relDir)
		}
	}
}

func TestPublicAPI_IntentionalTypeDuplicationStaysSmallAndExplicit(t *testing.T) {
	root := repositoryRoot(t)
	duplicates := collectDuplicateExportedTypeNamesByPackage(t, root, publicAPIPackages)
	wantDuplicates := map[string][]string{
		"Provider":   {"providers/edgetts", "providers/volcengine", "tts"},
		"VoiceExtra": {"providers/edgetts", "providers/volcengine"},
	}

	if len(duplicates) != len(wantDuplicates) {
		t.Fatalf("duplicate exported public type names = %v, want %v", duplicates, wantDuplicates)
	}
	for typeName, wantOwners := range wantDuplicates {
		gotOwners, ok := duplicates[typeName]
		if !ok {
			t.Fatalf("missing expected duplicate exported type %s in %v", typeName, duplicates)
		}
		if strings.Join(gotOwners, ",") != strings.Join(wantOwners, ",") {
			t.Fatalf("duplicate owners for %s = %v, want %v", typeName, gotOwners, wantOwners)
		}
	}
}

// AC-14: provider 差异封装在 provider 实现和专属选项中，不回流核心抽象。
func TestCoreTTSPackage_RemainsProviderAgnostic(t *testing.T) {
	root := repositoryRoot(t)
	files := collectNonTestGoSourceFiles(t, root, "tts")
	providerTokens := []string{"edgetts", "volcengine"}

	for _, file := range files {
		for _, imported := range file.Imported {
			if strings.HasPrefix(imported, "github.com/simp-lee/ttsbridge/providers/") {
				t.Fatalf("core tts package must not import provider package %s in %s", imported, file.RelPath)
			}
			if strings.HasPrefix(imported, "github.com/simp-lee/ttsbridge/internal/") {
				t.Fatalf("core tts package must not import internal package %s in %s", imported, file.RelPath)
			}
		}
	}

	for _, symbol := range collectExportedSymbols(t, root, "tts") {
		if lowerContainsAny(symbol.Name, providerTokens) {
			t.Fatalf("core exported %s %s should remain provider-agnostic", symbol.Kind, symbol.Name)
		}
	}
}
