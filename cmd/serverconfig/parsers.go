// Package main 将 Delphi 服务端配置文件转换为 JSONC 格式。
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

// ParseINI 解析 INI 格式文件，返回各节（section）到键值对的映射。
// 支持 [Section] 节头和 Key=Value 行。
// 以 ; 或 # 开头的行视为注释。
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

		// 跳过空行和注释
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}

		// 节头
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

// ParseSQLite 打开 SQLite 数据库文件并返回数据库连接。
func ParseSQLite(filename string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, fmt.Errorf("opening database %s: %w", filename, err)
	}
	return db, nil
}

// ParseCustomTable 解析自定义分隔符文件，以映射形式返回各行。
// 第一行视为表头行。
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

		// 跳过空行和注释
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}

		parts := strings.Split(line, separator)

		// 第一个非注释行是表头
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

// ParseLineList 将文件解析为非空、非注释行的列表。
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

// ReadGBKFile 读取 GBK 编码文件并转换为 UTF-8。
// 先尝试以 UTF-8 读取，失败则回退到 GBK 解码。
func ReadGBKFile(filename string) ([]byte, error) {
	// 先尝试以 UTF-8 读取
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	// 检查数据是否为有效 UTF-8
	if isValidUTF8(data) {
		return data, nil
	}

	// 回退到 GBK 解码
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

// isValidUTF8 检查数据是否为有效 UTF-8。
func isValidUTF8(data []byte) bool {
	// 检查常见的无效 UTF-8 序列
	// 这是简化检查——仅验证数据能否按 UTF-8 解码
	for i := 0; i < len(data); {
		b := data[i]
		if b < 0x80 {
			i++
			continue
		}
		if b < 0xC0 {
			return false // 无效的起始字节
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

// ReadUTF8File 读取 UTF-8 编码文件。
func ReadUTF8File(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

// WriteJSONC 将数据写为 JSONC 文件，可选添加头部注释。
func WriteJSONC(filename string, data string, comment string) error {
	// 确保目录存在
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("creating file %s: %w", filename, err)
	}
	defer f.Close()

	// 写入注释头
	if comment != "" {
		for _, line := range strings.Split(comment, "\n") {
			fmt.Fprintf(f, "// %s\n", line)
		}
		fmt.Fprintln(f)
	}

	// 写入数据
	_, err = f.WriteString(data)
	return err
}

// StringSliceToJSON 将字符串切片转换为 JSON 数组格式。
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

// MapSliceToJSON 将映射切片转换为 JSON 数组格式。
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

// CopyFile 将文件从 src 复制到 dst。
func CopyFile(src, dst string) error {
	// 确保目标目录存在
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

// CopyDir 将 srcDir 中匹配 pattern 的所有文件复制到 dstDir。
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

// CountLines 统计文件中非空、非注释行的数量。
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

// FileExists 检查文件是否存在。
func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

// DirExists 检查目录是否存在。
func DirExists(dirname string) bool {
	info, err := os.Stat(dirname)
	return err == nil && info.IsDir()
}
