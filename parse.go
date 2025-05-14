package ftp

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var errUnsupportedListLine = errors.New("unsupported LIST line")
var errUnsupportedListDate = errors.New("unsupported LIST date")
var errUnknownListEntryType = errors.New("unknown entry type")

type parseFunc func(string, time.Time, *time.Location) (*Entry, error)

var listLineParsers = []parseFunc{
	parseRFC3659ListLine,
	parseLsListLine,
	parseDirListLine,
	parseHostedFTPLine,
	parseIbmListLine,
}

var dirTimeFormats = []string{
	"01-02-06  03:04PM",
	"2006-01-02  15:04",
	"01-02-2006  03:04PM",
	"01-02-2006  15:04",
}

// parseRFC3659ListLine parses the style of directory line defined in RFC 3659.
func parseRFC3659ListLine(line string, _ time.Time, loc *time.Location) (*Entry, error) {
	return parseNextRFC3659ListLine(line, loc, &Entry{})
}

func parseNextRFC3659ListLine(line string, loc *time.Location, e *Entry) (*Entry, error) {
	iSemicolon := strings.Index(line, ";")
	iWhitespace := strings.Index(line, " ")

	if iSemicolon < 0 || iSemicolon > iWhitespace {
		return nil, errUnsupportedListLine
	}

	name := line[iWhitespace+1:]
	if e.Name == "" {
		e.Name = name
	} else if e.Name != name {
		// All lines must have the same name
		return nil, errUnsupportedListLine
	}

	for _, field := range strings.Split(line[:iWhitespace-1], ";") {
		i := strings.Index(field, "=")
		if i < 1 {
			return nil, errUnsupportedListLine
		}

		key := strings.ToLower(field[:i])
		value := field[i+1:]

		switch key {
		case "modify":
			var err error
			e.Time, err = time.ParseInLocation("20060102150405", value, loc)
			if err != nil {
				return nil, err
			}
		case "type":
			switch value {
			case "dir", "cdir", "pdir":
				e.Type = EntryTypeFolder
			case "file":
				e.Type = EntryTypeFile
			}
		case "size":
			if err := e.setSize(value); err != nil {
				return nil, err
			}
		}
	}
	return e, nil
}

// parseLsListLine parses a directory line in a format based on the output of
// the UNIX ls command.
func parseLsListLine(line string, now time.Time, loc *time.Location) (*Entry, error) {

	// Has the first field a length of exactly 10 bytes
	// - or 10 bytes with an additional '+' character for indicating ACLs?
	// If not, return.
	if i := strings.IndexByte(line, ' '); !(i == 10 || (i == 11 && line[10] == '+')) {
		return nil, errUnsupportedListLine
	}

	scanner := newScanner(line)
	fields := scanner.NextFields(6)

	if len(fields) < 6 {
		return nil, errUnsupportedListLine
	}

	if fields[1] == "folder" && fields[2] == "0" {
		e := &Entry{
			Type: EntryTypeFolder,
			Name: scanner.Remaining(),
		}
		if err := e.setTime(fields[3:6], now, loc); err != nil {
			return nil, err
		}

		return e, nil
	}

	if fields[1] == "0" {
		fields = append(fields, scanner.Next())
		e := &Entry{
			Type: EntryTypeFile,
			Name: scanner.Remaining(),
		}

		if err := e.setSize(fields[2]); err != nil {
			return nil, errUnsupportedListLine
		}
		if err := e.setTime(fields[4:7], now, loc); err != nil {
			return nil, err
		}

		return e, nil
	}

	// Read two more fields
	fields = append(fields, scanner.NextFields(2)...)
	if len(fields) < 8 {
		return nil, errUnsupportedListLine
	}

	e := &Entry{
		Name: scanner.Remaining(),
	}
	switch fields[0][0] {
	case '-':
		e.Type = EntryTypeFile
		if err := e.setSize(fields[4]); err != nil {
			return nil, err
		}
	case 'd':
		e.Type = EntryTypeFolder
	case 'l':
		e.Type = EntryTypeLink

		// Split link name and target
		if i := strings.Index(e.Name, " -> "); i > 0 {
			e.Target = e.Name[i+4:]
			e.Name = e.Name[:i]
		}
	default:
		return nil, errUnknownListEntryType
	}

	if err := e.setTime(fields[5:8], now, loc); err != nil {
		return nil, err
	}

	return e, nil
}

// parseDirListLine parses a directory line in a format based on the output of
// the MS-DOS DIR command.
func parseDirListLine(line string, now time.Time, loc *time.Location) (*Entry, error) {
	e := &Entry{}
	var err error

	// Try various time formats that DIR might use, and stop when one works.
	for _, format := range dirTimeFormats {
		if len(line) > len(format) {
			e.Time, err = time.ParseInLocation(format, line[:len(format)], loc)
			if err == nil {
				line = line[len(format):]
				break
			}
		}
	}
	if err != nil {
		// None of the time formats worked.
		return nil, errUnsupportedListLine
	}

	line = strings.TrimLeft(line, " ")
	if strings.HasPrefix(line, "<DIR>") {
		e.Type = EntryTypeFolder
		line = strings.TrimPrefix(line, "<DIR>")
	} else {
		space := strings.Index(line, " ")
		if space == -1 {
			return nil, errUnsupportedListLine
		}
		e.Size, err = strconv.ParseUint(line[:space], 10, 64)
		if err != nil {
			return nil, errUnsupportedListLine
		}
		e.Type = EntryTypeFile
		line = line[space:]
	}

	e.Name = strings.TrimLeft(line, " ")
	return e, nil
}

// parseHostedFTPLine parses a directory line in the non-standard format used
// by hostedftp.com
// -r--------   0 user group     65222236 Feb 24 00:39 UABlacklistingWeek8.csv
// (The link count is inexplicably 0)
func parseHostedFTPLine(line string, now time.Time, loc *time.Location) (*Entry, error) {
	// Has the first field a length of 10 bytes?
	if strings.IndexByte(line, ' ') != 10 {
		return nil, errUnsupportedListLine
	}

	scanner := newScanner(line)
	fields := scanner.NextFields(2)

	if len(fields) < 2 || fields[1] != "0" {
		return nil, errUnsupportedListLine
	}

	// Set link count to 1 and attempt to parse as Unix.
	return parseLsListLine(fields[0]+" 1 "+scanner.Remaining(), now, loc)
}

// parseIbmListLine parses a directory line in the format used by IBM systems (like AS/400, iSeries)
// Format example:
// TSTITFECOM        830 13/05/25 13:26:11 *STMF      ON_HAND_310325_000007.CSV
// TSTITFECOM        437 13/05/25 13:26:11 *STMF      ON_HAND__050005.CSV
// TSTITFECOM      12288 13/05/25 13:26:12 *DIR       .deleted/
// TSTITFECOM       8192 13/05/25 13:31:42 *DIR       114/
// TSTITFECOM       8192 13/05/25 13:31:42 *DIR       130/
// TSTITFECOM       8192 13/05/25 13:31:43 *DIR       561/
// TSTITFECOM      21504 13/05/25 13:31:42 *STMF      114/POLL53.DWN
// TSTITFECOM        644 13/05/25 13:31:42 *STMF      114/POLL79.DWN
// TSTITFECOM       1280 13/05/25 13:31:42 *STMF      114/POLLTM.DWN
func parseIbmListLine(line string, now time.Time, loc *time.Location) (*Entry, error) {
	// The format appears to have fixed columns
	scanner := newScanner(line)

	// First field is owner/username
	owner := scanner.Next()
	if owner == "" {
		return nil, errUnsupportedListLine
	}

	// Second field is the size
	sizeStr := scanner.Next()
	if sizeStr == "" {
		return nil, errUnsupportedListLine
	}

	// Next fields are date and time
	dateStr := scanner.Next()
	if dateStr == "" || len(dateStr) != 8 { // YY/MM/DD format
		return nil, errUnsupportedListLine
	}

	timeStr := scanner.Next()
	if timeStr == "" || len(timeStr) != 8 { // HH:MM:SS format
		return nil, errUnsupportedListLine
	}

	// Next field is file type
	fileType := scanner.Next()
	if fileType == "" {
		return nil, errUnsupportedListLine
	}

	// The rest is the path
	path := strings.TrimLeft(scanner.Remaining(), " ")
	if path == "" {
		return nil, errUnsupportedListLine
	}

	// Parse size
	size, err := strconv.ParseUint(sizeStr, 10, 64)
	if err != nil {
		return nil, err
	}

	// 13/05/25 is meant to be year=2025, month=05, day=13
	day, err := strconv.Atoi(dateStr[0:2])
	if err != nil {
		return nil, err
	}

	month, err := strconv.Atoi(dateStr[3:5])
	if err != nil {
		return nil, err
	}

	year, err := strconv.Atoi(dateStr[6:8])
	if err != nil {
		return nil, err
	}
	// Assuming YY is actually the full year 2025
	year = 2000 + year

	hour, err := strconv.Atoi(timeStr[0:2])
	if err != nil {
		return nil, err
	}

	min, err := strconv.Atoi(timeStr[3:5])
	if err != nil {
		return nil, err
	}

	sec, err := strconv.Atoi(timeStr[6:8])
	if err != nil {
		return nil, err
	}

	timestamp := time.Date(year, time.Month(month), day, hour, min, sec, 0, loc)

	// Determine entry type
	entryType := EntryTypeFile
	if fileType == "*DIR" {
		entryType = EntryTypeFolder
	}

	// Get the entry name from the path
	name := path
	if entryType == EntryTypeFolder && strings.HasSuffix(name, "/") {
		// Remove trailing slash for directory names
		name = name[:len(name)-1]
	}

	return &Entry{
		Name: name,
		Size: size,
		Time: timestamp,
		Type: entryType,
	}, nil
}

// parseListLine parses the various non-standard format returned by the LIST
// FTP command.
func parseListLine(line string, now time.Time, loc *time.Location) (*Entry, error) {
	for _, f := range listLineParsers {
		e, err := f(line, now, loc)
		if err != errUnsupportedListLine {
			return e, err
		}
	}
	return nil, errUnsupportedListLine
}

func (e *Entry) setSize(str string) (err error) {
	e.Size, err = strconv.ParseUint(str, 0, 64)
	return
}

func (e *Entry) setTime(fields []string, now time.Time, loc *time.Location) (err error) {
	if strings.Contains(fields[2], ":") { // contains time
		thisYear, _, _ := now.Date()
		timeStr := fmt.Sprintf("%s %s %d %s", fields[1], fields[0], thisYear, fields[2])
		e.Time, err = time.ParseInLocation("_2 Jan 2006 15:04", timeStr, loc)

		/*
			On unix, `info ls` shows:

			10.1.6 Formatting file timestamps
			---------------------------------

			A timestamp is considered to be “recent” if it is less than six
			months old, and is not dated in the future.  If a timestamp dated today
			is not listed in recent form, the timestamp is in the future, which
			means you probably have clock skew problems which may break programs
			like ‘make’ that rely on file timestamps.
		*/
		if !e.Time.Before(now.AddDate(0, 6, 0)) {
			e.Time = e.Time.AddDate(-1, 0, 0)
		}

	} else { // only the date
		if len(fields[2]) != 4 {
			return errUnsupportedListDate
		}
		timeStr := fmt.Sprintf("%s %s %s 00:00", fields[1], fields[0], fields[2])
		e.Time, err = time.ParseInLocation("_2 Jan 2006 15:04", timeStr, loc)
	}
	return
}
