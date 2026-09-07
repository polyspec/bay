package stack

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

func composeEnvironment(dir string) (map[string]string, error) {
	values := map[string]string{}
	// Shell values are present while evaluating .env references, and always win.
	for _, item := range os.Environ() {
		key, value, _ := strings.Cut(item, "=")
		values[key] = value
	}
	file, err := os.Open(filepath.Join(dir, ".env"))
	if os.IsNotExist(err) {
		return values, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		raw = strings.TrimPrefix(raw, "export ")
		key, value, ok := strings.Cut(raw, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || !variableName(key) {
			return nil, fmt.Errorf(".env line %d: expected NAME=value", line)
		}
		literal := strings.HasPrefix(value, "'")
		if literal {
			if len(value) < 2 || !strings.HasSuffix(value, "'") {
				return nil, fmt.Errorf(".env line %d: unterminated quote", line)
			}
			value = value[1 : len(value)-1]
		} else {
			if strings.HasPrefix(value, "\"") {
				value, err = strconv.Unquote(value)
				if err != nil {
					return nil, fmt.Errorf(".env line %d: invalid quoted value", line)
				}
			} else if i := strings.Index(value, " #"); i >= 0 {
				value = strings.TrimSpace(value[:i])
			}
			value, err = interpolate(value, values, 0)
			if err != nil {
				return nil, fmt.Errorf(".env line %d: %w", line, err)
			}
		}
		if _, shell := os.LookupEnv(key); !shell {
			values[key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading .env: %w", err)
	}
	return values, nil
}

func variableName(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func interpolateValues(node *yaml.Node, values map[string]string) error {
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		value, err := interpolate(node.Value, values, 0)
		if err != nil {
			return err
		}
		node.Value = value
	}
	for i, child := range node.Content {
		if node.Kind == yaml.MappingNode && i%2 == 0 {
			continue
		}
		if err := interpolateValues(child, values); err != nil {
			return err
		}
	}
	return nil
}

func interpolate(input string, values map[string]string, depth int) (string, error) {
	if depth > 32 {
		return "", fmt.Errorf("interpolation nesting exceeds 32")
	}
	var out strings.Builder
	for i := 0; i < len(input); {
		if input[i] != '$' {
			out.WriteByte(input[i])
			i++
			continue
		}
		i++
		if i < len(input) && input[i] == '$' {
			out.WriteByte('$')
			i++
			continue
		}
		if i < len(input) && input[i] == '{' {
			start, level := i+1, 1
			i++
			for i < len(input) && level > 0 {
				if input[i] == '{' {
					level++
				}
				if input[i] == '}' {
					level--
				}
				i++
			}
			if level != 0 {
				return "", fmt.Errorf("unterminated interpolation")
			}
			expr := input[start : i-1]
			n := 0
			for n < len(expr) && (expr[n] == '_' || expr[n] >= 'a' && expr[n] <= 'z' || expr[n] >= 'A' && expr[n] <= 'Z' || expr[n] >= '0' && expr[n] <= '9') {
				n++
			}
			key, op := expr[:n], expr[n:]
			if !variableName(key) {
				return "", fmt.Errorf("invalid interpolation variable")
			}
			value, set := values[key]
			if op != "" {
				test := set
				if strings.HasPrefix(op, ":") {
					test = set && value != ""
					op = op[1:]
				}
				if op == "" {
					return "", fmt.Errorf("invalid interpolation operator for %s", key)
				}
				kind, word := op[0], op[1:]
				switch kind {
				case '?':
					if !test {
						return "", fmt.Errorf("required environment variable %s is missing", key)
					}
				case '-':
					if !test {
						var err error
						value, err = interpolate(word, values, depth+1)
						if err != nil {
							return "", err
						}
					}
				case '+':
					value = ""
					if test {
						var err error
						value, err = interpolate(word, values, depth+1)
						if err != nil {
							return "", err
						}
					}
				default:
					return "", fmt.Errorf("unsupported interpolation operator for %s", key)
				}
			}
			out.WriteString(value)
			continue
		}
		start := i
		for i < len(input) && (input[i] == '_' || input[i] >= 'a' && input[i] <= 'z' || input[i] >= 'A' && input[i] <= 'Z' || i > start && input[i] >= '0' && input[i] <= '9') {
			i++
		}
		if start == i {
			return "", fmt.Errorf("literal dollar must be escaped as $$")
		}
		out.WriteString(values[input[start:i]])
	}
	return out.String(), nil
}
