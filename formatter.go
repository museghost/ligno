package ligno

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"
	"time"

	isatty "github.com/mattn/go-isatty"
)

const (
	floatFormat    = 'f'
	errorKey = "PARSE_ERROR"
)

// Formatter is interface for converting log record to string representation.
type Formatter interface {
	Format(record Record) []byte
}

// FormatterFunc is function type that implements Formatter interface.
type FormatterFunc func(Record) []byte

// Format is implementation of Formatter interface. It just calls function.
func (ff FormatterFunc) Format(record Record) []byte {
	return ff(record)
}

// DefaultTimeFormat is default time format.
const DefaultTimeFormat = time.RFC3339Nano

// SimpleFormat returns formatter that formats record with bare minimum of information.
// Intention of this formatter is to simulate standard library formatter.
func SimpleFormat() Formatter {
	return FormatterFunc(func(record Record) []byte {
		buff := buffPool.Get()
		defer buffPool.Put(buff)
		buff.WriteString(record.Time.Format(DefaultTimeFormat))
		buff.WriteRune(' ')
		buff.WriteString(record.Message)
		buff.WriteRune(' ')
		if record.File != "" && record.Line > 0 {
			buff.WriteRune('[')
			buff.WriteString(record.File)
			buff.WriteRune(':')
			buff.WriteString(strconv.Itoa(record.Line))
			buff.WriteRune(']')
		}
		buff.WriteRune('\n')
		return buff.Bytes()
	})
}

// TerminalFormat returns ThemeTerminalFormat with default theme set.
func TerminalFormat() Formatter {
	if isatty.IsTerminal(os.Stdout.Fd()) {
		return ThemedTerminalFormat(DefaultTheme)
	}
	return ThemedTerminalFormat(NoColorTheme)
}

// ThemedTerminalFormat returns formatter that produces records formatted for
// easy reading in terminal, but that are a bit richer then SimpleFormat (this
// one includes context keys)
func ThemedTerminalFormat(theme Theme) Formatter {
	return FormatterFunc(func(record Record) []byte {
		//time := record.Time.Format(DefaultTimeFormat)
		buff := buffPool.Get()
		defer buffPool.Put(buff)
		//buff.WriteString(theme.Time(time))
		//buff.WriteRune(' ')
		levelColor := theme.ForLevel(record.Level)
		levelName := record.Level.String()
		buff.WriteString(levelColor(levelName))
		padSpaces := levelNameMaxLength - len(levelName) + 2
		buff.Write(bytes.Repeat([]byte(" "), padSpaces))
		buff.WriteRune(' ')

		buff.WriteString(record.Message)

		record.Pairs = append(record.ContextList, record.Pairs...)
		record.Pairs = append([]interface{}{
			"ts", record.Time,
			"lvl", record.Level,
			"msg", record.Message},
			record.Pairs...)

		if len(record.Pairs) > 0 {
			buff.WriteString(" [")
		}
		for i := 0; i < len(record.Pairs); i += 2 {
			k := record.Pairs[i].(string)
			keyQuote := strings.IndexFunc(k, needsQuote) >= 0 || k == ""
			if keyQuote {
				buff.WriteRune('"')
			}
			buff.WriteString(k)
			if keyQuote {
				buff.WriteRune('"')
			}
			buff.WriteRune('=')
			buff.WriteRune('"')
			buff.WriteString(fmt.Sprintf("%+v", record.Pairs[i+1]))
			buff.WriteRune('"')
			if i < len(record.Pairs)-2 {
				buff.WriteRune(' ')
			}
		}
		if len(record.Pairs) > 0 {
			buff.WriteRune(']')
		}
		buff.WriteRune('\n')
		return buff.Bytes()
	})
}

// Needs quote determines if provided rune is such that word that contains this
// rune needs to be quoted.
func needsQuote(r rune) bool {
	return r == ' ' || r == '"' || r == '\\' || r == '=' ||
		!unicode.IsPrint(r)
}

// JSONFormat is simple formatter that only marshals log record to json.
func JSONFormat(pretty bool) Formatter {
	return FormatterFunc(func(record Record) []byte {
		// since errors are not JSON serializable, make sure that all errors
		// are converted to strings
		for idx := range record.ContextList {
			record.ContextList[idx] = fmt.Sprintf("%+v", record.ContextList[idx])
		}

		// set context (static value)
		record.Pairs = append(record.ContextList, record.Pairs...)

		// set default info
		record.Pairs = append([]interface{}{
			"ts", record.Time,
			"lvl", record.Level,
			"msg", record.Message},
			record.Pairs...)

		if record.Line > 0 {
			record.Pairs = append(record.Pairs, []interface{}{
				"file", record.File,
				"line", record.Line,
			}...)
		}

		tmpMap := make(map[string]interface{})
		for i := 0; i < len(record.Pairs); i += 2 {
			tmpMap[record.Pairs[i].(string)] = record.Pairs[i+1]
		}

		// serialize
		var marshaled []byte
		var err error
		if pretty {
			marshaled, err = json.MarshalIndent(tmpMap, "", "    ")
		} else {
			marshaled, err = json.Marshal(tmpMap)
		}
		if err != nil {
			marshaled, _ = json.Marshal(map[string]string{
				"JSONError": err.Error(),
			})
		}
		marshaled = append(marshaled, '\n')
		return marshaled
	})
}

func LogFmtFormat() Formatter {
	return FormatterFunc(func(record Record) []byte {
		record.Pairs = append(record.ContextList, record.Pairs...)

		// set default info
		record.Pairs = append([]interface{}{
			"ts", record.Time,
			"lvl", record.Level,
			"msg", record.Message},
			record.Pairs...)

		if record.Line > 0 {
			record.Pairs = append(record.Pairs, []interface{}{
				"file", record.File,
				"line", record.Line,
			}...)
		}

		// encoding them according to logfmt
		buf := &bytes.Buffer{}
		for i := 0; i < len(record.Pairs); i += 2 {
			if i != 0 {
				buf.WriteByte(' ')
			}

			k, ok := record.Pairs[i].(string)
			v := formatLogfmtValue(record.Pairs[i+1])
			if !ok {
				k, v = errorKey, formatLogfmtValue(k)
			}

			buf.WriteString(k)
			buf.WriteByte('=')
			buf.WriteString(v)
		}

		buf.WriteByte('\n')
		return buf.Bytes()
	})
}

// formatValue formats a value for serialization
func formatLogfmtValue(value interface{}) string {
	if value == nil {
		return "nil"
	}

	if t, ok := value.(time.Time); ok {
		// Performance optimization: No need for escaping since the provided
		// timeFormat doesn't have any escape characters, and escaping is
		// expensive.
		return t.Format(DefaultTimeFormat)
	}
	//value = formatShared(value)
	switch v := value.(type) {
	case bool:
		return strconv.FormatBool(v)
	case float32:
		return strconv.FormatFloat(float64(v), floatFormat, 3, 64)
	case float64:
		return strconv.FormatFloat(v, floatFormat, 3, 64)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", value)
	case string:
		return escapeString(v)
	default:
		return escapeString(fmt.Sprintf("%+v", value))
	}
}

func escapeString(s string) string {
	needsQuotes := false
	needsEscape := false
	for _, r := range s {
		if r <= ' ' || r == '=' || r == '"' {
			needsQuotes = true
		}
		if r == '\\' || r == '"' || r == '\n' || r == '\r' || r == '\t' {
			needsEscape = true
		}
	}
	if needsEscape == false && needsQuotes == false {
		return s
	}

	e := buffPool.Get()

	e.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\', '"':
			e.WriteByte('\\')
			e.WriteByte(byte(r))
		case '\n':
			e.WriteString("\\n")
		case '\r':
			e.WriteString("\\r")
		case '\t':
			e.WriteString("\\t")
		default:
			e.WriteRune(r)
		}
	}
	e.WriteByte('"')
	var ret string
	if needsQuotes {
		ret = e.String()
	} else {
		ret = string(e.Bytes()[1 : e.Len()-1])
	}
	e.Reset()
	buffPool.Put(e)
	return ret
}
