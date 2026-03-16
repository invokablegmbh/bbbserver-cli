package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"bbbserver-cli/internal/api"
)

type Renderer struct {
	Mode   string
	Pretty bool
	Stdout io.Writer
	Stderr io.Writer
}

func New(mode string, pretty bool) Renderer {
	if mode == "" {
		mode = "human"
	}
	return Renderer{
		Mode:   strings.ToLower(mode),
		Pretty: pretty,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

func (r Renderer) Data(value any) error {
	if r.Mode == "json" {
		return r.writeJSON(value)
	}
	return r.writeHuman(value)
}

func (r Renderer) Error(err error) error {
	_, _ = fmt.Fprintln(r.Stderr, err.Error())
	if r.Mode != "json" {
		return nil
	}
	return r.writeJSON(map[string]any{"error": api.ToPublicError(err)})
}

func (r Renderer) writeJSON(value any) error {
	var data []byte
	var err error
	if r.Pretty {
		data, err = json.MarshalIndent(value, "", "  ")
	} else {
		data, err = json.Marshal(value)
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(r.Stdout, string(data))
	return err
}

func (r Renderer) writeHuman(value any) error {
	normalized := normalizeValue(value)

	if items, ok := asObjectList(normalized); ok && rowsAreFlat(items) {
		return writeTable(r.Stdout, items)
	}

	if object, ok := normalized.(map[string]any); ok {
		if isFlatMap(object) {
			return writeKeyValue(r.Stdout, object)
		}
		return writeObject(r.Stdout, object, 0)
	}

	if list, ok := normalized.([]any); ok {
		return writeList(r.Stdout, list, 0)
	}

	_, err := fmt.Fprintln(r.Stdout, formatScalar(normalized))
	return err
}

func normalizeValue(value any) any {
	if value == nil {
		return nil
	}

	if _, ok := value.(map[string]any); ok {
		return value
	}
	if _, ok := value.([]any); ok {
		return value
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}

	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return value
	}
	return normalized
}

func writeAny(w io.Writer, value any, indent int) error {
	value = normalizeValue(value)

	if object, ok := value.(map[string]any); ok {
		return writeObject(w, object, indent)
	}
	if list, ok := value.([]any); ok {
		return writeList(w, list, indent)
	}

	_, err := fmt.Fprintln(w, indentString(indent)+formatScalar(value))
	return err
}

func writeObject(w io.Writer, object map[string]any, indent int) error {
	if len(object) == 0 {
		_, err := fmt.Fprintln(w, indentString(indent)+"(empty)")
		return err
	}

	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	keys = orderedKeys(keys)

	for _, key := range keys {
		value := normalizeValue(object[key])
		if isScalar(value) {
			if _, err := fmt.Fprintf(w, "%s%s: %s\n", indentString(indent), key, formatScalar(value)); err != nil {
				return err
			}
			continue
		}

		if _, err := fmt.Fprintf(w, "%s%s:\n", indentString(indent), key); err != nil {
			return err
		}
		if err := writeAny(w, value, indent+2); err != nil {
			return err
		}
	}

	return nil
}

func writeList(w io.Writer, list []any, indent int) error {
	if len(list) == 0 {
		_, err := fmt.Fprintln(w, indentString(indent)+"(empty)")
		return err
	}

	for _, item := range list {
		value := normalizeValue(item)
		if isScalar(value) {
			if _, err := fmt.Fprintf(w, "%s- %s\n", indentString(indent), formatScalar(value)); err != nil {
				return err
			}
			continue
		}

		if _, err := fmt.Fprintf(w, "%s-\n", indentString(indent)); err != nil {
			return err
		}
		if err := writeAny(w, value, indent+2); err != nil {
			return err
		}
	}

	return nil
}

func writeTable(w io.Writer, rows []map[string]any) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "(empty)")
		return err
	}

	columnSet := make(map[string]struct{})
	for _, row := range rows {
		for key := range row {
			columnSet[key] = struct{}{}
		}
	}

	columns := make([]string, 0, len(columnSet))
	for key := range columnSet {
		columns = append(columns, key)
	}
	columns = orderedKeys(columns)

	tw := tabwriter.NewWriter(w, 0, 8, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, strings.Join(columns, "\t"))
	for _, row := range rows {
		parts := make([]string, 0, len(columns))
		for _, column := range columns {
			parts = append(parts, scalarForTable(normalizeValue(row[column])))
		}
		_, _ = fmt.Fprintln(tw, strings.Join(parts, "\t"))
	}
	return tw.Flush()
}

func writeKeyValue(w io.Writer, object map[string]any) error {
	if len(object) == 0 {
		_, err := fmt.Fprintln(w, "(empty)")
		return err
	}

	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	keys = orderedKeys(keys)

	tw := tabwriter.NewWriter(w, 0, 8, 2, ' ', 0)
	for _, key := range keys {
		_, _ = fmt.Fprintf(tw, "%s:\t%s\n", key, formatScalar(normalizeValue(object[key])))
	}
	return tw.Flush()
}

func asObjectList(value any) ([]map[string]any, bool) {
	list, ok := value.([]any)
	if ok {
		rows := make([]map[string]any, 0, len(list))
		for _, item := range list {
			m, ok := normalizeValue(item).(map[string]any)
			if !ok {
				return nil, false
			}
			rows = append(rows, m)
		}
		if len(rows) > 0 {
			return rows, true
		}
	}

	if wrapped, ok := value.(map[string]any); ok {
		if response, exists := wrapped["response"]; exists {
			return asObjectList(response)
		}
	}

	return nil, false
}

func rowsAreFlat(rows []map[string]any) bool {
	for _, row := range rows {
		if !isFlatMap(row) {
			return false
		}
	}
	return true
}

func isFlatMap(object map[string]any) bool {
	for _, value := range object {
		if !isScalar(normalizeValue(value)) {
			return false
		}
	}
	return true
}

func isScalar(value any) bool {
	switch value.(type) {
	case nil, string, bool, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return true
	default:
		return false
	}
}

func scalarForTable(value any) string {
	if isScalar(value) {
		return formatScalar(value)
	}
	compact, err := json.Marshal(value)
	if err != nil {
		return formatScalar(value)
	}
	return string(compact)
}

func formatScalar(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		if typed {
			return "yes"
		}
		return "no"
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprintf("%v", value)
	}
}

func indentString(indent int) string {
	if indent <= 0 {
		return ""
	}
	return strings.Repeat(" ", indent)
}

func orderedKeys(keys []string) []string {
	ordered := make([]string, len(keys))
	copy(ordered, keys)

	sort.SliceStable(ordered, func(i, j int) bool {
		pi := keyPriority(ordered[i])
		pj := keyPriority(ordered[j])
		if pi != pj {
			return pi < pj
		}
		return ordered[i] < ordered[j]
	})

	return ordered
}

func keyPriority(key string) int {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "id":
		return 0
	case "name":
		return 1
	default:
		return 2
	}
}
