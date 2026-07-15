// Package main converts Delphi server configuration files to JSONC format.
package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// ParseINI parses an INI-style file and returns a map of sections to key-value pairs.
// Supports [Section] headers and Key=Value lines.
// Lines starting with ; or # are treated as comments.
func ParseINI(filename string) (map[string]map[string]string, error) {
	data, err := ReadGBKFile(filename)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filename, err)
	}

	result := make(map[string]map[string]string)
	currentSection := ""

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}

		// Section header
		if line[0] == '[' && line[len(line)-1] == ']' {
			currentSection = line[1 : len(line)-1]
			if _, exists := result[currentSection]; !exists {
				result[currentSection] = make(map[string]string)
			}
			continue
		}

		// Key=Value
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			if currentSection != "" {
				result[currentSection][key] = value
			}
		}
	}

	return result, nil
}

// ParseSQLite opens a SQLite database file and returns the database connection.
func ParseSQLite(filename string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, fmt.Errorf("opening database %s: %w", filename, err)
	}
	return db, nil
}

// ParseCustomTable parses a custom-delimited file and returns rows as maps.
// The first line is treated as the header row.
func ParseCustomTable(filename string, separator string) ([]map[string]string, error) {
	data, err := ReadGBKFile(filename)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filename, err)
	}

	var result []map[string]string
	var headers []string

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	lineNum := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lineNum++

		// Skip empty lines and comments
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}

		parts := strings.Split(line, separator)

		// First non-comment line is header
		if headers == nil {
			headers = make([]string, len(parts))
			for i, h := range parts {
				headers[i] = strings.TrimSpace(h)
			}
			continue
		}

		row := make(map[string]string)
		for i, part := range parts {
			if i < len(headers) {
				row[headers[i]] = strings.TrimSpace(part)
			}
		}
		result = append(result, row)
	}

	return result, nil
}

// ParseLineList parses a file into a list of non-empty, non-comment lines.
func ParseLineList(filename string) ([]string, error) {
	data, err := ReadGBKFile(filename)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filename, err)
	}

	var result []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}
		result = append(result, line)
	}

	return result, nil
}

// ReadGBKFile reads a GBK-encoded file and converts it to UTF-8.
// It first tries to read as UTF-8, and falls back to GBK if that fails.
func ReadGBKFile(filename string) ([]byte, error) {
	// First try reading as UTF-8
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	// Check if the data is valid UTF-8
	if isValidUTF8(data) {
		return data, nil
	}

	// Fall back to GBK decoding
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := transform.NewReader(f, simplifiedchinese.GBK.NewDecoder())
	data, err = io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("decoding GBK from %s: %w", filename, err)
	}

	return data, nil
}

// isValidUTF8 checks if the data is valid UTF-8.
func isValidUTF8(data []byte) bool {
	// Check for common invalid UTF-8 sequences
	// This is a simplified check - just verify the data can be decoded as UTF-8
	for i := 0; i < len(data); {
		b := data[i]
		if b < 0x80 {
			i++
			continue
		}
		if b < 0xC0 {
			return false // Invalid start byte
		}
		if b < 0xE0 {
			if i+1 >= len(data) || data[i+1]&0xC0 != 0x80 {
				return false
			}
			i += 2
			continue
		}
		if b < 0xF0 {
			if i+2 >= len(data) || data[i+1]&0xC0 != 0x80 || data[i+2]&0xC0 != 0x80 {
				return false
			}
			i += 3
			continue
		}
		if b < 0xF8 {
			if i+3 >= len(data) || data[i+1]&0xC0 != 0x80 || data[i+2]&0xC0 != 0x80 || data[i+3]&0xC0 != 0x80 {
				return false
			}
			i += 4
			continue
		}
		return false
	}
	return true
}

// ReadUTF8File reads a UTF-8 encoded file.
func ReadUTF8File(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

// WriteJSONC writes data as a JSONC file with optional header comments.
func WriteJSONC(filename string, data string, comment string) error {
	// Ensure directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("creating file %s: %w", filename, err)
	}
	defer f.Close()

	// Write comment header
	if comment != "" {
		for _, line := range strings.Split(comment, "\n") {
			fmt.Fprintf(f, "// %s\n", line)
		}
		fmt.Fprintln(f)
	}

	// Write data
	_, err = f.WriteString(data)
	return err
}

// StringSliceToJSON converts a string slice to JSON array format.
func StringSliceToJSON(items []string) string {
	var sb strings.Builder
	sb.WriteString("[\n")
	for i, item := range items {
		if i > 0 {
			sb.WriteString(",\n")
		}
		sb.WriteString(fmt.Sprintf("  %q", item))
	}
	sb.WriteString("\n]")
	return sb.String()
}

// MapSliceToJSON converts a slice of maps to JSON array format.
func MapSliceToJSON(items []map[string]string) string {
	var sb strings.Builder
	sb.WriteString("[\n")
	for i, item := range items {
		if i > 0 {
			sb.WriteString(",\n")
		}
		sb.WriteString("  {\n")
		j := 0
		for k, v := range item {
			if j > 0 {
				sb.WriteString(",\n")
			}
			sb.WriteString(fmt.Sprintf("    %q: %q", k, v))
			j++
		}
		sb.WriteString("\n  }")
	}
	sb.WriteString("\n]")
	return sb.String()
}

// CopyFile copies a file from src to dst.
func CopyFile(src, dst string) error {
	// Ensure destination directory exists
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source file %s: %w", src, err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("creating destination file %s: %w", dst, err)
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// CopyDir copies all files matching pattern from srcDir to dstDir.
func CopyDir(srcDir, dstDir, pattern string) (int, error) {
	matches, err := filepath.Glob(filepath.Join(srcDir, pattern))
	if err != nil {
		return 0, fmt.Errorf("globbing pattern %s: %w", pattern, err)
	}

	count := 0
	for _, src := range matches {
		dst := filepath.Join(dstDir, filepath.Base(src))
		if err := CopyFile(src, dst); err != nil {
			return count, err
		}
		count++
	}

	return count, nil
}

// CountLines counts the number of non-empty, non-comment lines in a file.
func CountLines(filename string) (int, error) {
	data, err := ReadGBKFile(filename)
	if err != nil {
		return 0, err
	}

	count := 0
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && line[0] != ';' && line[0] != '#' {
			count++
		}
	}
	return count, nil
}

// FileExists checks if a file exists.
func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

// DirExists checks if a directory exists.
func DirExists(dirname string) bool {
	info, err := os.Stat(dirname)
	return err == nil && info.IsDir()
}
