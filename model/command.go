package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// pre commands here
const activeBucket = "active"

// post commands here
// will delete after sync to server
const archivedBucket = "archived"

type CommandPhase int

const (
	CommandPhasePre  = 0
	CommandPhasePost = 1
)

type Command struct {
	Shell     string       `json:"shell"`
	SessionID int64        `json:"sid"`
	Command   string       `json:"cmd"`
	Main      string       `json:"main"`
	Hostname  string       `json:"hn"`
	Username  string       `json:"un"`
	Time      time.Time    `json:"t"`
	EndTime   time.Time    `json:"et"`
	Result    int          `json:"result"`
	Phase     CommandPhase `json:"phase"`
	PPID      int          `json:"ppid,omitempty"`
	// ViaSSH is set when the shell is an SSH login. Nil on records written
	// before it was captured, which the server stores as unknown.
	ViaSSH *bool `json:"ssh,omitempty"`

	// Only work in file
	RecordingTime time.Time `json:"-"`
}

// privateFileMode keeps command history, cursors, and queued sync payloads
// readable only by the owning user. Those files can contain secrets that were
// typed into the shell.
const privateFileMode os.FileMode = 0o600

func ensureStorageFolder() error {
	storageFolder := GetCommandsStoragePath()
	if _, err := os.Stat(storageFolder); os.IsNotExist(err) {
		if err := os.MkdirAll(storageFolder, 0o700); err != nil {
			return fmt.Errorf("failed to create command storage folder: %v", err)
		}
	}

	return nil
}

// openPrivateAppend opens path for append, creating it if needed, and forces
// owner-only permissions. OpenFile's mode is masked by umask and does not
// change an existing file.
func openPrivateAppend(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, privateFileMode)
	if err != nil {
		return nil, err
	}
	if err := f.Chmod(privateFileMode); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// WritePrivateFile writes data and forces owner-only permissions, including
// when the file already exists (WriteFile leaves an existing mode unchanged).
func WritePrivateFile(path string, data []byte) error {
	if err := os.WriteFile(path, data, privateFileMode); err != nil {
		return err
	}
	return os.Chmod(path, privateFileMode)
}

func (c Command) DoSavePre() error {
	if err := ensureStorageFolder(); err != nil {
		return err
	}

	buf, err := c.ToLine(time.Now())
	if err != nil {
		return err
	}

	preFile := GetPreCommandFilePath()
	f, err := openPrivateAppend(preFile)
	if err != nil {
		slog.Error("failed to open pre-command storage file", slog.Any("err", err))
		return fmt.Errorf("failed to open pre-command storage file: %v", err)
	}
	defer f.Close()

	if _, err := f.Write(buf); err != nil {
		slog.Error("failed to write to pre-command storage file", slog.Any("err", err))
		return fmt.Errorf("failed to write to pre-command storage file: %v", err)
	}
	return nil
}

func (c Command) DoUpdate(result int) error {
	if err := ensureStorageFolder(); err != nil {
		return err
	}

	c.Phase = CommandPhasePost
	c.Result = result
	c.EndTime = time.Now()
	buf, err := c.ToLine(time.Now())
	if err != nil {
		return err
	}

	postFile := GetPostCommandFilePath()
	f, err := openPrivateAppend(postFile)
	if err != nil {
		return fmt.Errorf("failed to open post-command storage file: %v", err)
	}
	defer f.Close()

	if _, err := f.Write(buf); err != nil {
		return fmt.Errorf("failed to write to post-command storage file: %v", err)
	}
	return nil
}

func (c Command) IsPairPreCommand(target Command) bool {
	if !c.IsSame(target) {
		return false
	}
	if c.Phase != CommandPhasePre {
		return false
	}
	// one command not possible to run for 10 days, right?
	if c.Time.Before(time.Now().Add(-time.Hour * 24 * 10)) {
		return false
	}
	return true
}

func (c Command) IsSame(target Command) bool {
	if c.Shell != target.Shell {
		return false
	}
	if c.Command != target.Command {
		return false
	}
	if c.SessionID != target.SessionID {
		return false
	}
	if c.Username != target.Username {
		return false
	}
	return true
}

func (c Command) IsNil() bool {
	if c.Command != "" {
		return false
	}
	if c.SessionID != 0 {
		return false
	}
	if c.Username != "" {
		return false
	}
	if c.Shell != "" {
		return false
	}
	return true
}

func (cmd Command) GetUniqueKey() string {
	return fmt.Sprintf("%s|%d|%s|%s", cmd.Shell, cmd.SessionID, cmd.Command, cmd.Username)
}

func (cmd Command) ToLine(recordingTime time.Time) (buf []byte, err error) {
	buf, err = json.Marshal(cmd)
	if err != nil {
		return
	}
	timestamp := recordingTime.UnixNano()
	timestampBytes := []byte(fmt.Sprintf("%d", timestamp))
	buf = append(buf, SEPARATOR)
	buf = append(buf, timestampBytes...)
	buf = append(buf, '\n')
	return
}

func (cmd *Command) FromLine(line string) (recordingTime time.Time, err error) {
	parts := strings.Split(line, string(SEPARATOR))
	if len(parts) != 2 {
		err = fmt.Errorf("Invalid line format in pre-command file: %s", line)
		slog.Error("invalid line format", slog.String("line", line))
		return
	}

	err = json.Unmarshal([]byte(parts[0]), cmd)
	if err != nil {
		err = fmt.Errorf("failed to unmarshal command: %v, %s", err, parts[0])
		slog.Error("failed to unmarshal command", slog.Any("err", err))
		return
	}

	unixNano, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		err = fmt.Errorf("failed to parse timestamp: %v, %s", err, parts[1])
		slog.Error("failed to parse timestamp", slog.Any("err", err))
		return
	}
	recordingTime = time.Unix(0, unixNano)
	cmd.RecordingTime = recordingTime
	return
}

func (cmd *Command) FromLineBytes(line []byte) (recordingTime time.Time, err error) {
	parts := bytes.Split(line, []byte{SEPARATOR})
	if len(parts) != 2 {
		err = fmt.Errorf("Invalid line format in FromLineBytes: %s", string(line))
		slog.Error("invalid line format", slog.String("line", string(line)))
		return
	}

	err = json.Unmarshal(parts[0], cmd)
	if err != nil {
		err = fmt.Errorf("failed to unmarshal command: %v, %s, %s", err, string(parts[0]), string(line))
		slog.Error("failed to unmarshal command", slog.Any("err", err))
		return
	}

	unixNano, err := strconv.ParseInt(string(parts[1]), 10, 64)
	if err != nil {
		err = fmt.Errorf("failed to parse timestamp: %v, %s", err, string(parts[1]))
		slog.Error("failed to parse timestamp", slog.Any("err", err))
		return
	}
	recordingTime = time.Unix(0, unixNano)
	cmd.RecordingTime = recordingTime
	return
}

func (cmd Command) FindClosestCommand(commandList []*Command, withSameKey bool) *Command {
	var closestPreCommand *Command
	minTimeDiff := int64(^uint64(0) >> 1) // Max int64 value

	myKey := cmd.GetUniqueKey()

	for _, c := range commandList {
		if c == nil {
			continue
		}
		if withSameKey {
			ck := c.GetUniqueKey()
			if myKey != ck {
				continue
			}
		}

		// Whole seconds collapse two commands that ran in the same second onto
		// the same diff, so the first candidate wins instead of the closest.
		timeDiff := cmd.Time.UnixNano() - c.Time.UnixNano()
		if timeDiff >= 0 && timeDiff < minTimeDiff {
			minTimeDiff = timeDiff
			closestPreCommand = c
		}
	}

	if closestPreCommand == nil {
		// Callers historically check IsNil() on a non-nil result.
		return new(Command)
	}
	return closestPreCommand
}
