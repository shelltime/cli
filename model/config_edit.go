package model

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
	"gopkg.in/yaml.v3"
)

// ConfigBool is one boolean setting for WriteConfigBools. Path holds its keys from the top of
// the config file, e.g. ai, agent, view.
type ConfigBool struct {
	Path  []string
	Value bool
}

// WriteConfigBools writes boolean settings to the config files in configDir and returns the
// files it wrote. A setting goes to config.local.* when that file has the setting's top-level
// section, which then overrides the base file's (mergeConfig), else to the base file. Only the
// lines a setting needs change, so comments, blank lines and the order of keys stay as the
// user wrote them.
func WriteConfigBools(configDir string, settings []ConfigBool) ([]string, error) {
	files := findConfigFiles(configDir)
	if files.baseFile == "" {
		return nil, fmt.Errorf("%w in %s", ErrConfigNotFound, configDir)
	}
	// ReadConfigFile skips a local file that doesn't load as a config (a syntax error, or a
	// value of the wrong type), so such a file decides nothing here either
	var local *ShellTimeConfig
	if files.localFile != "" {
		if data, err := os.ReadFile(files.localFile); err == nil {
			var config ShellTimeConfig
			if unmarshalConfig(data, files.localFormat, &config) == nil {
				local = &config
			}
		}
	}

	var order []string
	byFile := map[string][]ConfigBool{}
	for _, s := range settings {
		file := files.baseFile
		if local != nil && setsSection(*local, files.localFormat, s.Path[0]) {
			file = files.localFile
		}
		if _, seen := byFile[file]; !seen {
			order = append(order, file)
		}
		byFile[file] = append(byFile[file], s)
	}

	for _, file := range order {
		if err := writeConfigBoolsToFile(file, byFile[file]); err != nil {
			return nil, err
		}
	}
	return order, nil
}

// setsSection reports whether config, decoded from a local file, sets its top-level section key,
// which mergeConfig then takes over the base file's. It reads the field the decoder filled, so
// a section left empty (`ai:`) sets nothing, and TOML's case-insensitive keys count too.
func setsSection(config ShellTimeConfig, format configFormat, key string) bool {
	v := reflect.ValueOf(config)
	for i := 0; i < v.NumField(); i++ {
		name, _, _ := strings.Cut(v.Type().Field(i).Tag.Get(string(format)), ",")
		if name != key {
			continue
		}
		switch field := v.Field(i); field.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Map:
			return !field.IsNil()
		default:
			return !field.IsZero()
		}
	}
	return false
}

func writeConfigBoolsToFile(file string, settings []ConfigBool) error {
	// Write through a symlink (a config kept in a dotfiles repo) rather than replace it
	real, err := filepath.EvalSymlinks(file)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	info, err := os.Stat(real)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	original, err := os.ReadFile(real)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	data := original
	format := detectFormat(file)
	for _, s := range settings {
		data, err = setConfigBool(data, format, s.Path, s.Value)
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
	}
	if bytes.Equal(data, original) {
		return nil
	}
	return replaceFileKeepingMode(real, data, info.Mode().Perm())
}

// replaceFileKeepingMode replaces path with data and gives it perm, so a failed write never
// leaves a truncated config behind. The new file is the writer's, without the old one's
// extended attributes or other hard links. A folder that refuses a new file (a read-only
// dotfiles checkout holding a writable config) gets the file rewritten in place instead; any
// other failure (a full disk) changes nothing, since an in-place write could then leave the
// config cut short.
func replaceFileKeepingMode(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		if !errors.Is(err, fs.ErrPermission) {
			return fmt.Errorf("failed to write config file: %w", err)
		}
		if writeErr := os.WriteFile(path, data, perm); writeErr != nil {
			return fmt.Errorf("failed to write config file: %w", writeErr)
		}
		return nil
	}
	defer os.Remove(tmp.Name()) // fails harmlessly once renamed

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to write config file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	if err := os.Chmod(tmp.Name(), perm); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

// setConfigBool returns the config file content data with the boolean at path set to value.
// It tries the edits from the smallest up and keeps the first that changes nothing else and
// still loads as a config.
func setConfigBool(data []byte, format configFormat, path []string, value bool) ([]byte, error) {
	var edits [][]byte
	var err error
	if format == formatYAML {
		edits, err = yamlBoolEdits(data, path, value)
	} else {
		edits, err = tomlBoolEdits(data, path, value)
	}
	if err != nil {
		return nil, err
	}
	for _, edited := range edits {
		if onlyConfigBoolChanged(data, edited, format, path, value) {
			return edited, nil
		}
	}
	return nil, fmt.Errorf("couldn't set %s in this file; set it to %t there by hand",
		strings.Join(path, "."), value)
}

// onlyConfigBoolChanged reports whether edited decodes to original with the boolean at path set
// to value, and nothing else changed.
func onlyConfigBoolChanged(original, edited []byte, format configFormat, path []string, value bool) bool {
	want, err := decodeConfigMap(original, format)
	if err != nil {
		return false
	}
	got, err := decodeConfigMap(edited, format)
	if err != nil {
		return false
	}
	var config ShellTimeConfig
	if err := unmarshalConfig(edited, format, &config); err != nil {
		return false
	}
	setMapPath(want, path, value, format == formatTOML)
	return reflect.DeepEqual(want, got)
}

func decodeConfigMap(data []byte, format configFormat) (map[string]any, error) {
	var m map[string]any
	var err error
	if format == formatYAML {
		err = yaml.Unmarshal(data, &m)
	} else {
		err = toml.Unmarshal(data, &m)
	}
	if err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// setMapPath sets the boolean at path in m. With fold, a key already there in another case is
// the one set, as go-toml reads TOML keys in any case.
func setMapPath(m map[string]any, path []string, value bool, fold bool) {
	spelled := func(m map[string]any, key string) string {
		if fold {
			for existing := range m {
				if strings.EqualFold(existing, key) {
					return existing
				}
			}
		}
		return key
	}
	for _, key := range path[:len(path)-1] {
		key = spelled(m, key)
		next, ok := m[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[key] = next
		}
		m = next
	}
	m[spelled(m, path[len(path)-1])] = value
}

// yamlBoolEdits returns candidate edits of a YAML config: the value or the missing keys
// patched into the text, then the whole document encoded again, which keeps comments but not
// blank lines (for flow mappings, quoted values and other shapes the patch leaves alone; never
// for a file of several documents).
func yamlBoolEdits(data []byte, path []string, value bool) ([][]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	var edits [][]byte
	if edited, ok := patchYAMLBool(data, &doc, path, value); ok {
		edits = append(edits, edited)
	}
	// Encoding keeps only the first document, and the check after it reads only that one
	if yamlDocuments(data) <= 1 {
		if edited, ok := reencodeYAMLBool(data, path, value); ok {
			edits = append(edits, edited)
		}
	}
	return edits, nil
}

func yamlDocuments(data []byte) int {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	count := 0
	for {
		var doc yaml.Node
		if err := dec.Decode(&doc); err != nil {
			// io.EOF, or a later document that doesn't parse: count it so nothing is dropped
			if !errors.Is(err, io.EOF) {
				count++
			}
			return count
		}
		count++
	}
}

func patchYAMLBool(data []byte, doc *yaml.Node, path []string, value bool) ([]byte, bool) {
	lit := strconv.FormatBool(value)
	nl := lineBreak(data)
	unit := yamlIndentUnit(doc)

	var node *yaml.Node
	if len(doc.Content) > 0 {
		node = doc.Content[0]
	}
	if node == nil || (node.Kind == yaml.ScalarNode && node.Tag == "!!null") {
		// An empty file, or one with only comments
		return insertText(data, len(data), yamlBlock(path, 0, unit, lit, nl), nl), true
	}

	var parent *yaml.Node // the key node is the value of
	for i, key := range path {
		if parent != nil && yamlEmptyValue(node) {
			return yamlFillEmpty(data, parent, node, yamlBlock(path[i:], parent.Column-1+unit, unit, lit, nl), nl)
		}
		if node.Kind != yaml.MappingNode || node.Style&yaml.FlowStyle != 0 {
			return nil, false
		}
		k, v := yamlEntry(node, key)
		if k == nil {
			// A block mapping has at least one entry; add the rest of the path after its last
			first := node.Content[0]
			indent := first.Column - 1
			at := yamlMappingEnd(data, first.Line, indent)
			return insertText(data, at, yamlBlock(path[i:], indent, unit, lit, nl), nl), true
		}
		if i == len(path)-1 {
			// Any plain value (false, null, ~); an empty one has no text to replace
			if v.Kind != yaml.ScalarNode || v.Style != 0 || v.Value == "" {
				return nil, false
			}
			start, ok := yamlOffset(data, v.Line, v.Column)
			if !ok || !bytes.HasPrefix(data[start:], []byte(v.Value)) {
				return nil, false
			}
			return splice(data, start, start+len(v.Value), lit), true
		}
		parent, node = k, v
	}
	return nil, false
}

// yamlEmptyValue reports whether node is a section left empty: {}, null, ~ or nothing at all,
// as `shelltime init` writes `agent: {}`.
func yamlEmptyValue(node *yaml.Node) bool {
	return (node.Kind == yaml.MappingNode && node.Style&yaml.FlowStyle != 0 && len(node.Content) == 0) ||
		(node.Kind == yaml.ScalarNode && node.Tag == "!!null")
}

// yamlFillEmpty replaces the empty value of key with block, the keys under it on the lines
// that follow.
func yamlFillEmpty(data []byte, key, value *yaml.Node, block, nl string) ([]byte, bool) {
	token := value.Value
	if value.Kind == yaml.MappingNode {
		token = "{}"
	}
	keyStart, ok := yamlOffset(data, key.Line, key.Column)
	if !ok {
		return nil, false
	}
	if token == "" {
		// `ai:` with nothing after it
		return insertText(data, lineEnd(data, keyStart), block, nl), true
	}
	start, ok := yamlOffset(data, value.Line, value.Column)
	if !ok || value.Line != key.Line || !bytes.HasPrefix(data[start:], []byte(token)) {
		return nil, false
	}
	from := start
	for from > keyStart && data[from-1] == ' ' {
		from--
	}
	out := splice(data, from, start+len(token), "")
	return insertText(out, lineEnd(out, keyStart), block, nl), true
}

func reencodeYAMLBool(data []byte, path []string, value bool) ([]byte, bool) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
		return nil, false
	}
	unit := yamlIndentUnit(&doc)

	node := doc.Content[0]
	for i, key := range path {
		if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
			node.Kind, node.Tag, node.Value, node.Style = yaml.MappingNode, "!!map", "", 0
		}
		if node.Kind != yaml.MappingNode {
			return nil, false
		}
		k, v := yamlEntry(node, key)
		if k == nil {
			v = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
		}
		if i == len(path)-1 {
			v.Kind, v.Tag, v.Value, v.Style, v.Content = yaml.ScalarNode, "!!bool", strconv.FormatBool(value), 0, nil
		}
		node = v
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(unit)
	if err := enc.Encode(&doc); err != nil {
		return nil, false
	}
	if err := enc.Close(); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

func yamlEntry(mapping *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i], mapping.Content[i+1]
		}
	}
	return nil, nil
}

// yamlIndentUnit is how far the file indents a nested mapping: 2 unless a nested block mapping
// shows otherwise (yaml.Marshal, which `shelltime init` writes with, indents by 4).
func yamlIndentUnit(node *yaml.Node) int {
	if node.Kind == yaml.MappingNode && node.Style&yaml.FlowStyle == 0 {
		for i := 0; i+1 < len(node.Content); i += 2 {
			k, v := node.Content[i], node.Content[i+1]
			if v.Kind == yaml.MappingNode && v.Style&yaml.FlowStyle == 0 && len(v.Content) > 0 {
				if unit := v.Content[0].Column - k.Column; unit > 0 {
					return unit
				}
			}
		}
	}
	for _, child := range node.Content {
		if unit := yamlIndentUnit(child); unit != 2 {
			return unit
		}
	}
	return 2
}

// yamlBlock writes path as nested keys starting at indent, the last one set to lit.
func yamlBlock(path []string, indent, unit int, lit, nl string) string {
	var b strings.Builder
	for i, key := range path {
		b.WriteString(strings.Repeat(" ", indent+i*unit))
		b.WriteString(key)
		b.WriteString(":")
		if i == len(path)-1 {
			b.WriteString(" " + lit)
		}
		b.WriteString(nl)
	}
	return b.String()
}

// yamlMappingEnd returns the offset just past the last line of the block mapping whose first
// key is on line firstLine (1-based) at indent. Comments and blank lines after it belong to
// what follows, and a document marker (--- or ...) ends it.
func yamlMappingEnd(data []byte, firstLine, indent int) int {
	end, offset := len(data), 0
	for i, line := range bytes.SplitAfter(data, []byte("\n")) {
		offset += len(line)
		if i+1 < firstLine {
			continue
		}
		text := strings.TrimRight(string(line), "\r\n")
		trimmed := strings.TrimLeft(text, " ")
		if strings.TrimSpace(trimmed) == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if len(text)-len(trimmed) < indent || yamlDocumentMarker(text) {
			break
		}
		end = offset
	}
	return end
}

func yamlDocumentMarker(line string) bool {
	for _, marker := range []string{"---", "..."} {
		if rest, ok := strings.CutPrefix(line, marker); ok && (rest == "" || rest[0] == ' ' || rest[0] == '\t') {
			return true
		}
	}
	return false
}

// yamlOffset converts a yaml.v3 position (1-based line, 1-based column in characters) to a
// byte offset.
func yamlOffset(data []byte, line, column int) (int, bool) {
	offset := 0
	for l := 1; l < line; l++ {
		i := bytes.IndexByte(data[offset:], '\n')
		if i < 0 {
			return 0, false
		}
		offset += i + 1
	}
	for c := 1; c < column; c++ {
		if offset >= len(data) || data[offset] == '\n' {
			return 0, false
		}
		_, size := utf8.DecodeRune(data[offset:])
		offset += size
	}
	return offset, true
}

// tomlBoolEdits returns candidate edits of a TOML config: the value replaced where it is set,
// else the key added under the deepest [table] it belongs to, as a new table at the end of the
// file, or as a dotted key at the top.
func tomlBoolEdits(data []byte, path []string, value bool) ([][]byte, error) {
	lit := strconv.FormatBool(value)
	nl := lineBreak(data)

	type header struct {
		path []string
		end  int // just past the header's line
	}
	var headers []header
	var table []string
	inArrayTable := false

	var p unstable.Parser
	p.Reset(data)
	for p.NextExpression() {
		e := p.Expression()
		switch e.Kind {
		case unstable.Table, unstable.ArrayTable:
			var last *unstable.Node
			table, last = tomlKey(e.Key())
			inArrayTable = e.Kind == unstable.ArrayTable
			if !inArrayTable {
				headers = append(headers, header{table, lineEnd(data, int(last.Raw.Offset+last.Raw.Length))})
			}
		case unstable.KeyValue:
			if inArrayTable {
				continue
			}
			if start, end, ok := tomlFindBool(&p, e, table, path); ok {
				return [][]byte{splice(data, start, end, lit)}, nil
			}
		}
	}
	if err := p.Error(); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	var edits [][]byte
	best := -1
	for i, h := range headers {
		if len(h.path) < len(path) && tomlPathHasPrefix(path, h.path) &&
			(best < 0 || len(h.path) > len(headers[best].path)) {
			best = i
		}
	}
	if best >= 0 {
		h := headers[best]
		line := strings.Join(path[len(h.path):], ".") + " = " + lit + nl
		edits = append(edits, insertText(data, h.end, line, nl))
	}
	if len(path) > 1 {
		block := "[" + strings.Join(path[:len(path)-1], ".") + "]" + nl + path[len(path)-1] + " = " + lit + nl
		if len(bytes.TrimSpace(data)) > 0 {
			block = nl + block
		}
		edits = append(edits, insertText(data, len(data), block, nl))
	}
	edits = append(edits, splice(data, 0, 0, strings.Join(path, ".")+" = "+lit+nl))
	return edits, nil
}

// tomlFindBool finds the boolean value of path in the key-value kv of table, looking into
// inline tables, and returns its byte range.
func tomlFindBool(p *unstable.Parser, kv *unstable.Node, table, path []string) (int, int, bool) {
	keys, _ := tomlKey(kv.Key())
	full := append(slices.Clone(table), keys...)
	value := kv.Value()
	if len(full) == len(path) && tomlPathHasPrefix(path, full) {
		if value.Kind != unstable.Bool {
			return 0, 0, false
		}
		r := p.Range(value.Data)
		return int(r.Offset), int(r.Offset + r.Length), true
	}
	if value.Kind == unstable.InlineTable && tomlPathHasPrefix(path, full) {
		children := value.Children()
		for children.Next() {
			if start, end, ok := tomlFindBool(p, children.Node(), full, path); ok {
				return start, end, true
			}
		}
	}
	return 0, 0, false
}

func tomlKey(it unstable.Iterator) ([]string, *unstable.Node) {
	var parts []string
	var last *unstable.Node
	for it.Next() {
		last = it.Node()
		parts = append(parts, string(last.Data))
	}
	return parts, last
}

// tomlPathHasPrefix compares keys in any case, as go-toml matches them to the config's fields.
func tomlPathHasPrefix(path, prefix []string) bool {
	return len(prefix) <= len(path) && slices.EqualFunc(path[:len(prefix)], prefix, strings.EqualFold)
}

// lineEnd returns the offset just past the line break at or after from.
func lineEnd(data []byte, from int) int {
	if i := bytes.IndexByte(data[from:], '\n'); i >= 0 {
		return from + i + 1
	}
	return len(data)
}

func lineBreak(data []byte) string {
	if bytes.Contains(data, []byte("\r\n")) {
		return "\r\n"
	}
	return "\n"
}

// insertText inserts text at offset, first ending the line before it if it isn't.
func insertText(data []byte, offset int, text, nl string) []byte {
	if offset > 0 && data[offset-1] != '\n' {
		text = nl + text
	}
	return splice(data, offset, offset, text)
}

func splice(data []byte, start, end int, text string) []byte {
	out := make([]byte, 0, len(data)-(end-start)+len(text))
	out = append(out, data[:start]...)
	out = append(out, text...)
	return append(out, data[end:]...)
}
