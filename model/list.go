package model

import (
	"context"
	"time"
)

// ListedCommand is a paired pre/post command for display by `shelltime ls`.
// JSON tags match the historical `ls -f json` output.
type ListedCommand struct {
	Command   string    `json:"command"`
	Shell     string    `json:"shell"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Result    int       `json:"result"`
	Username  string    `json:"username"`
	Hostname  string    `json:"hostname"`
}

// BuildListedCommands pairs each post command with its closest pre command and
// returns the rows for display. Shared by the CLI (file store) and the daemon
// (bolt store, queried over the socket) so both produce identical output.
func BuildListedCommands(ctx context.Context, store CommandStore) ([]ListedCommand, error) {
	postCommands, err := store.GetPostCommands(ctx)
	if err != nil {
		return nil, err
	}
	preTree, err := store.GetPreTree(ctx)
	if err != nil {
		return nil, err
	}

	commands := make([]ListedCommand, 0, len(postCommands))
	for _, postCommand := range postCommands {
		if postCommand == nil {
			continue
		}
		key := postCommand.GetUniqueKey()
		preCommands, ok := preTree[key]
		if !ok {
			continue
		}

		closestPreCommand := postCommand.FindClosestCommand(preCommands, false)
		startTime := postCommand.Time
		if closestPreCommand != nil {
			startTime = closestPreCommand.Time
		}

		commands = append(commands, ListedCommand{
			Command:   postCommand.Command,
			Shell:     postCommand.Shell,
			StartTime: startTime,
			EndTime:   postCommand.Time,
			Result:    postCommand.Result,
			Username:  postCommand.Username,
			Hostname:  postCommand.Hostname,
		})
	}
	return commands, nil
}
