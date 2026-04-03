package configgen

import (
	"bufio"
	"os"
	"strings"
)

// EnvEntry represents a single key=value pair from a .env file.
type EnvEntry struct {
	Key   string
	Value string
	Line  int // 1-based line number in the file
}

// ParseEnvFile reads a .env file and returns all key-value entries.
// It skips blank lines and comment lines (starting with #).
// Values may be optionally quoted with single or double quotes (quotes are stripped).
// Splits on the first '=' only — values may contain '=' characters.
func ParseEnvFile(path string) ([]EnvEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []EnvEntry
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on the first '=' only.
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue // no '=' found, skip malformed line
		}

		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])

		// Strip matching quotes.
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		entries = append(entries, EnvEntry{
			Key:   key,
			Value: value,
			Line:  lineNum,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}
