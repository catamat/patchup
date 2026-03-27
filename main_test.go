package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fixedNow() time.Time {
	return time.Date(2026, 3, 25, 14, 20, 0, 0, time.UTC)
}

func createTempFile(t *testing.T, content string) string {
	t.Helper()

	return createTempFileWithMode(t, content, 0o600)
}

func createTempFileWithMode(t *testing.T, content string, mode os.FileMode) string {
	t.Helper()

	fileName := filepath.Join(t.TempDir(), "version.go")
	if err := os.WriteFile(fileName, []byte(content), mode); err != nil {
		t.Fatal(err)
	}

	return fileName
}

func readFile(t *testing.T, fileName string) string {
	t.Helper()

	content, err := os.ReadFile(fileName)
	if err != nil {
		t.Fatal(err)
	}

	return string(content)
}

func TestIncrementPatchVersion(t *testing.T) {
	initialContent := `package main

const versionPatch = "1"
`

	expectedContent := `package main

const versionPatch = "2"
`

	fileName := createTempFile(t, initialContent)
	if err := run([]string{"-fn", fileName, "-pn", "versionPatch"}, fixedNow); err != nil {
		t.Fatal(err)
	}

	if result := readFile(t, fileName); result != expectedContent {
		t.Fatalf("expected %q, got %q", expectedContent, result)
	}
}

func TestUpdateTimestamp(t *testing.T) {
	initialContent := `package main

const versionTimestamp = "0000000000"
`

	expectedContent := `package main

const versionTimestamp = "2603251420"
`

	fileName := createTempFile(t, initialContent)
	if err := run([]string{"-fn", fileName, "-pn", "", "-tn", "versionTimestamp", "-tf", "0601021504"}, fixedNow); err != nil {
		t.Fatal(err)
	}

	if result := readFile(t, fileName); result != expectedContent {
		t.Fatalf("expected %q, got %q", expectedContent, result)
	}
}

func TestMissingPatchTargetReturnsError(t *testing.T) {
	initialContent := `package main

const unrelatedVar = "1"
const anotherVar = "0000000000"
`

	fileName := createTempFile(t, initialContent)
	err := run([]string{"-fn", fileName, "-pn", "versionPatch"}, fixedNow)
	if err == nil {
		t.Fatal("expected an error")
	}

	if !strings.Contains(err.Error(), `patch target "versionPatch" not found at package scope`) {
		t.Fatalf("unexpected error: %v", err)
	}

	if result := readFile(t, fileName); result != initialContent {
		t.Fatalf("expected file to remain unchanged, got %q", result)
	}
}

func TestRunCanBeCalledRepeatedly(t *testing.T) {
	initialContent := `package main

const versionPatch = "1"
`

	expectedContent := `package main

const versionPatch = "3"
`

	fileName := createTempFile(t, initialContent)
	for i := 0; i < 2; i++ {
		if err := run([]string{"-fn", fileName, "-pn", "versionPatch"}, fixedNow); err != nil {
			t.Fatal(err)
		}
	}

	if result := readFile(t, fileName); result != expectedContent {
		t.Fatalf("expected %q, got %q", expectedContent, result)
	}
}

func TestPatchReferenceBeforeDeclarationDoesNotTouchOtherLiterals(t *testing.T) {
	initialContent := `package main

const version = versionPatch + "."
const versionPatch = "1"
`

	expectedContent := `package main

const version = versionPatch + "."
const versionPatch = "2"
`

	fileName := createTempFile(t, initialContent)
	if err := run([]string{"-fn", fileName, "-pn", "versionPatch"}, fixedNow); err != nil {
		t.Fatal(err)
	}

	if result := readFile(t, fileName); result != expectedContent {
		t.Fatalf("expected %q, got %q", expectedContent, result)
	}
}

func TestOnlyPackageScopePatchDeclarationIsUpdated(t *testing.T) {
	initialContent := `package main

const versionPatch = "1"

func version() string {
	const versionPatch = "41"
	return versionPatch
}
`

	expectedContent := `package main

const versionPatch = "2"

func version() string {
	const versionPatch = "41"
	return versionPatch
}
`

	fileName := createTempFile(t, initialContent)
	if err := run([]string{"-fn", fileName, "-pn", "versionPatch"}, fixedNow); err != nil {
		t.Fatal(err)
	}

	if result := readFile(t, fileName); result != expectedContent {
		t.Fatalf("expected %q, got %q", expectedContent, result)
	}
}

func TestTimestampReferenceBeforeDeclarationDoesNotTouchOtherLiterals(t *testing.T) {
	initialContent := `package main

const version = versionTimestamp + "suffix"
const versionTimestamp = "old"
`

	expectedContent := `package main

const version = versionTimestamp + "suffix"
const versionTimestamp = "2603251420"
`

	fileName := createTempFile(t, initialContent)
	if err := run([]string{"-fn", fileName, "-pn", "", "-tn", "versionTimestamp", "-tf", "0601021504"}, fixedNow); err != nil {
		t.Fatal(err)
	}

	if result := readFile(t, fileName); result != expectedContent {
		t.Fatalf("expected %q, got %q", expectedContent, result)
	}
}

func TestNonNumericPatchReturnsError(t *testing.T) {
	initialContent := `package main

const versionPatch = "x"
`

	fileName := createTempFile(t, initialContent)
	err := run([]string{"-fn", fileName, "-pn", "versionPatch"}, fixedNow)
	if err == nil {
		t.Fatal("expected an error")
	}

	if !strings.Contains(err.Error(), "versionPatch must contain a numeric string") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseFlagsHelpReturnsUsageError(t *testing.T) {
	_, err := parseFlags([]string{"-h"})
	if err == nil {
		t.Fatal("expected an error")
	}

	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp, got %v", err)
	}

	var usageErr *usageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected usageError, got %T", err)
	}

	if !strings.Contains(usageErr.usage, "Usage of patchup:") {
		t.Fatalf("expected usage header, got %q", usageErr.usage)
	}

	if !strings.Contains(usageErr.usage, "-fn") {
		t.Fatalf("expected flags in usage, got %q", usageErr.usage)
	}
}

func TestParseFlagsInvalidFlagReturnsSingleErrorMessage(t *testing.T) {
	_, err := parseFlags([]string{"-badflag"})
	if err == nil {
		t.Fatal("expected an error")
	}

	if count := strings.Count(err.Error(), "flag provided but not defined: -badflag"); count != 1 {
		t.Fatalf("expected invalid flag message once, got %d in %q", count, err.Error())
	}
}

func TestRunPreservesFileMode(t *testing.T) {
	initialContent := `package main

const versionPatch = "1"
`

	fileName := createTempFileWithMode(t, initialContent, 0o754)
	if err := run([]string{"-fn", fileName, "-pn", "versionPatch"}, fixedNow); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(fileName)
	if err != nil {
		t.Fatal(err)
	}

	if got := info.Mode().Perm(); got != 0o754 {
		t.Fatalf("expected mode 0754, got %o", got)
	}
}

func TestRunPreservesCRLFLineEndings(t *testing.T) {
	initialContent := "package main\r\n\r\nconst versionPatch = \"1\"\r\n"
	expectedContent := "package main\r\n\r\nconst versionPatch = \"2\"\r\n"

	fileName := createTempFile(t, initialContent)
	if err := run([]string{"-fn", fileName, "-pn", "versionPatch"}, fixedNow); err != nil {
		t.Fatal(err)
	}

	if result := readFile(t, fileName); result != expectedContent {
		t.Fatalf("expected %q, got %q", expectedContent, result)
	}
}

func TestRunUpdatesSymlinkTargetWithoutReplacingLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges on Windows")
	}

	dir := t.TempDir()
	targetFile := filepath.Join(dir, "real-version.go")
	linkFile := filepath.Join(dir, "version.go")

	initialContent := `package main

const versionPatch = "1"
`

	expectedContent := `package main

const versionPatch = "2"
`

	if err := os.WriteFile(targetFile, []byte(initialContent), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(targetFile, linkFile); err != nil {
		t.Fatal(err)
	}

	if err := run([]string{"-fn", linkFile, "-pn", "versionPatch"}, fixedNow); err != nil {
		t.Fatal(err)
	}

	info, err := os.Lstat(linkFile)
	if err != nil {
		t.Fatal(err)
	}

	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("expected version.go to remain a symlink")
	}

	if result := readFile(t, targetFile); result != expectedContent {
		t.Fatalf("expected target file content %q, got %q", expectedContent, result)
	}

	if result := readFile(t, linkFile); result != expectedContent {
		t.Fatalf("expected symlink content %q, got %q", expectedContent, result)
	}
}
