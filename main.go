package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func main() {
	if err := run(os.Args[1:], time.Now); err != nil {
		var usageErr *usageError
		if errors.Is(err, flag.ErrHelp) {
			if errors.As(err, &usageErr) && usageErr.usage != "" {
				fmt.Fprint(os.Stdout, usageErr.usage)
			}
			os.Exit(0)
		}

		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type options struct {
	fileName         string
	patchName        string
	requirePatch     bool
	timestampName    string
	requireTimestamp bool
	timestampFormat  string
}

type usageError struct {
	err   error
	usage string
}

func (e *usageError) Error() string {
	if e.usage == "" {
		return e.err.Error()
	}

	return fmt.Sprintf("%v\n\n%s", e.err, e.usage)
}

func (e *usageError) Unwrap() error {
	return e.err
}

func run(args []string, now func() time.Time) error {
	opts, err := parseFlags(args)
	if err != nil {
		return err
	}

	if now == nil {
		now = time.Now
	}

	setFile := token.NewFileSet()
	astFile, err := parser.ParseFile(setFile, opts.fileName, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	changed, err := updateVersionFile(
		astFile,
		opts.patchName,
		opts.requirePatch,
		opts.timestampName,
		opts.requireTimestamp,
		now().UTC().Format(opts.timestampFormat),
	)
	if err != nil {
		return err
	}

	if !changed {
		return nil
	}

	return writeVersionFile(opts.fileName, setFile, astFile)
}

func parseFlags(args []string) (options, error) {
	fs := flag.NewFlagSet("patchup", flag.ContinueOnError)
	fs.SetOutput(&bytes.Buffer{})

	fileName := fs.String("fn", "version.go", "Go source file name")
	patchName := fs.String("pn", "versionPatch", "Patch var/const name")
	timestampName := fs.String("tn", "versionTimestamp", "Timestamp var/const name")
	timestampFormat := fs.String("tf", "0601021504", "Timestamp output format")

	if err := fs.Parse(args); err != nil {
		return options{}, &usageError{err: err, usage: flagUsage(fs)}
	}

	if *patchName == "" && *timestampName == "" {
		return options{}, &usageError{
			err:   errors.New("at least one of -pn or -tn must be non-empty"),
			usage: flagUsage(fs),
		}
	}

	if *patchName != "" && *patchName == *timestampName {
		return options{}, &usageError{
			err:   errors.New("-pn and -tn must refer to different names"),
			usage: flagUsage(fs),
		}
	}

	var explicitTimestampName bool
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "tn" {
			explicitTimestampName = true
		}
	})

	return options{
		fileName:         *fileName,
		patchName:        *patchName,
		requirePatch:     *patchName != "",
		timestampName:    *timestampName,
		requireTimestamp: *timestampName != "" && (explicitTimestampName || *patchName == ""),
		timestampFormat:  *timestampFormat,
	}, nil
}

func flagUsage(fs *flag.FlagSet) string {
	var usage bytes.Buffer
	fmt.Fprintf(&usage, "Usage of %s:\n", fs.Name())

	prevOutput := fs.Output()
	fs.SetOutput(&usage)
	fs.PrintDefaults()
	fs.SetOutput(prevOutput)

	return usage.String()
}

func updateVersionFile(
	astFile *ast.File,
	patchName string,
	requirePatch bool,
	timestampName string,
	requireTimestamp bool,
	timestamp string,
) (bool, error) {
	var (
		changed          bool
		patchMatches     int
		timestampMatches int
	)

	for _, decl := range astFile.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			result, err := updateValueSpec(valueSpec, patchName, timestampName, timestamp)
			if err != nil {
				return false, err
			}

			changed = changed || result.changed
			patchMatches += result.patchMatches
			timestampMatches += result.timestampMatches
		}
	}

	if err := validateMatchCount("patch", patchName, patchMatches, requirePatch); err != nil {
		return false, err
	}

	if err := validateMatchCount("timestamp", timestampName, timestampMatches, requireTimestamp); err != nil {
		return false, err
	}

	return changed, nil
}

func validateMatchCount(label, name string, count int, required bool) error {
	if name == "" {
		return nil
	}

	switch {
	case count == 0:
		if required {
			return fmt.Errorf("%s target %q not found at package scope", label, name)
		}
		return nil
	case count > 1:
		return fmt.Errorf("%s target %q matched %d declarations at package scope", label, name, count)
	default:
		return nil
	}
}

type valueSpecUpdateResult struct {
	changed          bool
	patchMatches     int
	timestampMatches int
}

func updateValueSpec(spec *ast.ValueSpec, patchName, timestampName, timestamp string) (valueSpecUpdateResult, error) {
	var result valueSpecUpdateResult

	for idx, name := range spec.Names {
		if patchName != "" && name.Name == patchName {
			result.patchMatches++

			specChanged, err := incrementStringLiteral(spec, idx, patchName)
			if err != nil {
				return valueSpecUpdateResult{}, err
			}

			result.changed = result.changed || specChanged
		}

		if timestampName != "" && name.Name == timestampName {
			result.timestampMatches++

			specChanged, err := setStringLiteral(spec, idx, timestampName, timestamp)
			if err != nil {
				return valueSpecUpdateResult{}, err
			}

			result.changed = result.changed || specChanged
		}
	}

	return result, nil
}

func incrementStringLiteral(spec *ast.ValueSpec, index int, fieldName string) (bool, error) {
	lit, value, err := valueSpecStringLiteral(spec, index, fieldName)
	if err != nil {
		return false, err
	}

	incrementedValue, err := incrementDecimalString(value)
	if err != nil {
		return false, fmt.Errorf("%s must contain only decimal digits", fieldName)
	}

	lit.Value = strconv.Quote(incrementedValue)
	return true, nil
}

func incrementDecimalString(value string) (string, error) {
	if value == "" {
		return "", errors.New("empty decimal string")
	}

	digits := []byte(value)
	for _, digit := range digits {
		if digit < '0' || digit > '9' {
			return "", errors.New("non-decimal digit")
		}
	}

	for i := len(digits) - 1; i >= 0; i-- {
		if digits[i] == '9' {
			digits[i] = '0'
			continue
		}

		digits[i]++
		return string(digits), nil
	}

	return "1" + string(digits), nil
}

func setStringLiteral(spec *ast.ValueSpec, index int, fieldName, value string) (bool, error) {
	lit, _, err := valueSpecStringLiteral(spec, index, fieldName)
	if err != nil {
		return false, err
	}

	quotedValue := strconv.Quote(value)
	if lit.Value == quotedValue {
		return false, nil
	}

	lit.Value = quotedValue
	return true, nil
}

func valueSpecStringLiteral(spec *ast.ValueSpec, index int, fieldName string) (*ast.BasicLit, string, error) {
	if index >= len(spec.Values) {
		return nil, "", fmt.Errorf("%s must have an explicit string literal value", fieldName)
	}

	lit, ok := spec.Values[index].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return nil, "", fmt.Errorf("%s must be assigned a string literal", fieldName)
	}

	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return nil, "", fmt.Errorf("%s has an invalid string literal: %w", fieldName, err)
	}

	return lit, value, nil
}

func writeVersionFile(fileName string, setFile *token.FileSet, astFile *ast.File) error {
	resolvedFileName, err := filepath.EvalSymlinks(fileName)
	if err != nil {
		return err
	}

	info, err := os.Stat(resolvedFileName)
	if err != nil {
		return err
	}

	content, err := formatVersionFile(resolvedFileName, setFile, astFile)
	if err != nil {
		return err
	}

	tempFile, err := os.CreateTemp(filepath.Dir(resolvedFileName), filepath.Base(resolvedFileName)+".tmp-*")
	if err != nil {
		return err
	}

	tempName := tempFile.Name()
	keepTempFile := false
	defer func() {
		if keepTempFile {
			return
		}

		tempFile.Close()
		_ = os.Remove(tempName)
	}()

	if err := tempFile.Chmod(info.Mode()); err != nil {
		return err
	}

	if _, err := tempFile.Write(content); err != nil {
		return err
	}

	if err := tempFile.Sync(); err != nil {
		return err
	}

	if err := tempFile.Close(); err != nil {
		return err
	}

	if err := os.Rename(tempName, resolvedFileName); err != nil {
		return err
	}

	keepTempFile = true
	return nil
}

func formatVersionFile(fileName string, setFile *token.FileSet, astFile *ast.File) ([]byte, error) {
	originalContent, err := os.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	var formatted bytes.Buffer
	if err := printer.Fprint(&formatted, setFile, astFile); err != nil {
		return nil, err
	}

	content := formatted.Bytes()
	if bytes.Contains(originalContent, []byte("\r\n")) {
		content = bytes.ReplaceAll(content, []byte("\n"), []byte("\r\n"))
	}

	return content, nil
}
