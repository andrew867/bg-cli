package output

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// WriteTable renders a compact textual view for terminal use.
// Complex nested payloads are summarized; prefer JSON for automation.
func WriteTable(w io.Writer, env Envelope) error {
	if !env.OK {
		if env.Error != nil {
			_, err := fmt.Fprintf(w, "error\t%s\t%s\n", env.Error.Code, env.Error.Message)
			return err
		}
		_, err := fmt.Fprintln(w, "error\tunknown")
		return err
	}
	_, _ = fmt.Fprintf(w, "command\t%s\n", env.Command)
	if env.Source != nil {
		_, _ = fmt.Fprintf(w, "endpoint\t%s\n", env.Source.Endpoint)
		_, _ = fmt.Fprintf(w, "httpStatus\t%d\n", env.Source.HTTPStatus)
		_, _ = fmt.Fprintf(w, "retrievedAt\t%s\n", env.Source.RetrievedAt.UTC().Format("2006-01-02T15:04:05Z"))
	}
	if env.Freshness != nil {
		_, _ = fmt.Fprintf(w, "freshness\t%s\n", env.Freshness.Meaning)
		if env.Freshness.SnapshotAt != nil {
			_, _ = fmt.Fprintf(w, "snapshotAt\t%s\n", env.Freshness.SnapshotAt.UTC().Format("2006-01-02T15:04:05Z"))
		}
	}
	return writeDataTable(w, "data", env.Data, 0)
}

func writeDataTable(w io.Writer, prefix string, v any, depth int) error {
	if depth > 3 {
		_, err := fmt.Fprintf(w, "%s\t…\n", prefix)
		return err
	}
	switch t := v.(type) {
	case nil:
		_, err := fmt.Fprintf(w, "%s\t\n", prefix)
		return err
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := writeDataTable(w, prefix+"."+k, t[k], depth+1); err != nil {
				return err
			}
		}
		return nil
	case []any:
		_, err := fmt.Fprintf(w, "%s.count\t%d\n", prefix, len(t))
		return err
	case string:
		_, err := fmt.Fprintf(w, "%s\t%s\n", prefix, sanitizeTableCell(t))
		return err
	case float64:
		// Avoid scientific notation for common integer-valued floats.
		if t == float64(int64(t)) && t < 1e15 && t > -1e15 {
			_, err := fmt.Fprintf(w, "%s\t%.0f\n", prefix, t)
			return err
		}
		_, err := fmt.Fprintf(w, "%s\t%f\n", prefix, t)
		return err
	case bool:
		_, err := fmt.Fprintf(w, "%s\t%t\n", prefix, t)
		return err
	default:
		_, err := fmt.Fprintf(w, "%s\t%v\n", prefix, t)
		return err
	}
}

func sanitizeTableCell(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
