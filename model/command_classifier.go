package model

import (
	"slices"
	"strings"
)

// CommandActionType represents the type of action a command performs
type CommandActionType string

const (
	ActionView   CommandActionType = "view"
	ActionEdit   CommandActionType = "edit"
	ActionDelete CommandActionType = "delete"
	ActionOther  CommandActionType = "other"
)

// ClassifyCommand analyzes a command line and determines its action type.
//
// Compound commands (pipelines, lists, command and process substitutions)
// are split into segments and the most severe segment wins. Multi-line
// scripts, unknown commands and anything that runs code through another
// interpreter or wrapper (sh, python, xargs, sudo, ...) are ActionOther,
// which is never auto-run.
func ClassifyCommand(command string) CommandActionType {
	cmd := strings.TrimSpace(command)
	if cmd == "" || strings.ContainsAny(cmd, "\n\r") {
		return ActionOther
	}

	segments := splitCommandSegments(cmd)
	if len(segments) == 0 {
		return ActionOther
	}
	result := ActionView
	for _, segment := range segments {
		action := classifySegment(segment)
		if action == ActionOther {
			return ActionOther
		}
		if actionSeverity(action) > actionSeverity(result) {
			result = action
		}
	}
	return result
}

func actionSeverity(a CommandActionType) int {
	switch a {
	case ActionView:
		return 1
	case ActionEdit:
		return 2
	case ActionDelete:
		return 3
	}
	return 0
}

// splitCommandSegments splits a command line on |, ||, &&, ;, &, subshell
// parentheses, $( ), backticks and <( ) / >( ), honoring quotes and
// backslash escapes. Redirections such as 2>&1 and &> are not split.
func splitCommandSegments(cmd string) []string {
	var segments []string
	var cur strings.Builder
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			segments = append(segments, s)
		}
		cur.Reset()
	}

	rs := []rune(cmd)
	inSingle, inDouble := false, false
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		next := rune(0)
		if i+1 < len(rs) {
			next = rs[i+1]
		}
		prev := rune(0)
		if i > 0 {
			prev = rs[i-1]
		}

		switch {
		case inSingle:
			if r == '\'' {
				inSingle = false
			}
			cur.WriteRune(r)
		case r == '\\' && next != 0:
			cur.WriteRune(r)
			cur.WriteRune(next)
			i++
		case r == '\'' && !inDouble:
			inSingle = true
			cur.WriteRune(r)
		case r == '"':
			inDouble = !inDouble
			cur.WriteRune(r)
		case r == '`':
			flush()
		case r == '$' && next == '(':
			flush()
			i++
		case inDouble:
			cur.WriteRune(r)
		case (r == '<' || r == '>') && next == '(':
			flush()
			i++
		case r == '&' && (prev == '>' || next == '>'):
			cur.WriteRune(r)
		case r == '|' || r == ';' || r == '&' || r == '(' || r == ')':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return segments
}

// runsArbitraryCode are commands that execute their arguments (or stdin) as
// code, so their effect cannot be classified from the command line.
var runsArbitraryCode = []string{
	"sh", "bash", "zsh", "fish", "dash", "ksh", "mksh", "csh", "tcsh", "nu", "pwsh", "powershell",
	"python", "python2", "python3", "node", "deno", "bun", "perl", "ruby", "php", "lua", "osascript",
	"eval", "exec", "source", ".", "xargs", "parallel", "sudo", "doas", "su", "nohup", "time",
	"timeout", "nice", "watch", "ssh", "builtin",
}

// classifySegment classifies a single simple command.
func classifySegment(segment string) CommandActionType {
	parts := strings.Fields(segment)
	if len(parts) == 0 {
		return ActionOther
	}
	mainCmd := parts[0]
	args := parts[1:]
	// ls>out.txt: the redirection is checked separately below
	if idx := strings.IndexAny(mainCmd, "<>"); idx > 0 {
		mainCmd = mainCmd[:idx]
	}

	// FOO=bar cmd: the assignment is harmless, classify the command
	if strings.Contains(mainCmd, "=") && !strings.HasPrefix(mainCmd, "=") {
		if len(args) == 0 {
			return ActionView
		}
		return classifySegment(strings.Join(args, " "))
	}

	// Commands given by path: classify by base name
	if strings.Contains(mainCmd, "/") {
		baseName := mainCmd[strings.LastIndex(mainCmd, "/")+1:]
		if baseName == "" {
			return ActionOther
		}
		return classifySegment(strings.Join(append([]string{baseName}, args...), " "))
	}

	action := classifyMainCommand(mainCmd, args)
	if action == ActionView && hasOutputRedirection(segment) {
		return ActionEdit
	}
	return action
}

func classifyMainCommand(mainCmd string, parts []string) CommandActionType {
	if slices.Contains(runsArbitraryCode, mainCmd) {
		return ActionOther
	}

	switch mainCmd {
	case "env", "command":
		// Bare `env` prints the environment and `command -v x` looks x up;
		// with other arguments both run a command.
		if len(parts) == 0 || (mainCmd == "command" && (parts[0] == "-v" || parts[0] == "-V")) {
			return ActionView
		}
		return ActionOther

	case "find":
		for _, p := range parts {
			switch p {
			case "-delete":
				return ActionDelete
			case "-exec", "-execdir", "-ok", "-okdir":
				return ActionOther
			}
		}
		return ActionView

	case "curl":
		for _, p := range parts {
			if p == "-O" || p == "--remote-name" || p == "--output" || p == "-K" || p == "--config" ||
				strings.HasPrefix(p, "-o") || strings.HasPrefix(p, "--output=") {
				return ActionEdit
			}
		}
		return ActionView

	case "wget":
		return ActionEdit

	case "git":
		return classifyGit(parts)

	case "docker", "podman":
		return classifyDocker(parts)

	case "kubectl":
		return classifyKubectl(parts)

	case "systemctl":
		return classifySystemctl(parts)

	// View commands
	case "cat", "less", "more", "head", "tail", "grep", "ls", "ll", "la", "echo", "printf",
		"ps", "top", "htop", "df", "du", "free", "netstat", "ss", "lsof",
		"which", "whereis", "file", "stat", "wc", "sort", "uniq",
		"cut", "paste", "join", "comm", "diff",
		"tree", "pwd", "whoami", "id", "groups", "hostname", "uname",
		"date", "cal", "uptime", "w", "who", "last", "history",
		"printenv", "set", "alias", "type",
		"man", "info", "help", "apropos", "whatis",
		"dig", "nslookup", "host", "ping", "traceroute",
		"journalctl", "dmesg":
		return ActionView

	// Edit commands
	case "vim", "vi", "nano", "emacs", "code", "subl", "atom", "gedit", "kate",
		"nvim", "neovim", "ed", "sed", "awk",
		"touch", "mkdir", "cp", "mv", "ln", "chmod", "chown", "chgrp",
		"tar", "zip", "unzip", "gzip", "gunzip", "bzip2", "bunzip2",
		"tee", "dd", "rsync", "scp", "sftp",
		"apt", "apt-get", "yum", "dnf", "pacman", "brew", "snap",
		"npm", "yarn", "pip", "pip3", "gem", "cargo", "go", "make", "cmake",
		"gcc", "g++", "clang", "java", "javac":
		// Check if it's a package manager installing/removing
		if isPackageManager(mainCmd) && len(parts) > 0 {
			switch parts[0] {
			case "remove", "uninstall", "purge", "autoremove":
				return ActionDelete
			}
		}
		return ActionEdit

	// Delete commands
	case "rm", "rmdir", "unlink", "shred", "truncate", "wipefs":
		return ActionDelete
	}

	return ActionOther
}

// gitViewSubcommands only read repository state.
var gitViewSubcommands = []string{
	"status", "log", "diff", "show", "blame", "describe", "rev-parse", "rev-list", "ls-files",
	"ls-tree", "ls-remote", "shortlog", "grep", "help", "version", "cat-file", "count-objects",
	"name-rev", "cherry", "for-each-ref", "show-ref", "whatchanged", "reflog",
}

func classifyGit(parts []string) CommandActionType {
	if len(parts) == 0 {
		return ActionView
	}
	sub, args := parts[0], parts[1:]
	// Global options such as -c core.pager=... or --exec-path can make any
	// subcommand run arbitrary programs
	if strings.HasPrefix(sub, "-") {
		return ActionOther
	}
	hasArg := func(names ...string) bool {
		return slices.ContainsFunc(args, func(a string) bool { return slices.Contains(names, a) })
	}
	onlyFlags := func(allowed ...string) bool {
		return !slices.ContainsFunc(args, func(a string) bool { return !slices.Contains(allowed, a) })
	}

	switch {
	case sub == "reflog" && len(args) > 0 && args[0] != "show":
		return ActionEdit
	case slices.Contains(gitViewSubcommands, sub):
		return ActionView
	case sub == "rm" || sub == "clean":
		return ActionDelete
	case sub == "reset" && hasArg("--hard"):
		return ActionDelete
	case sub == "branch" && hasArg("-d", "-D", "--delete"):
		return ActionDelete
	case sub == "branch" && onlyFlags("-a", "-r", "-v", "-vv", "--all", "--remotes", "--list", "--show-current"):
		return ActionView
	case sub == "stash" && len(args) > 0 && (args[0] == "drop" || args[0] == "clear"):
		return ActionDelete
	case sub == "stash" && len(args) > 0 && (args[0] == "list" || args[0] == "show"):
		return ActionView
	case sub == "tag" && hasArg("-d", "--delete"):
		return ActionDelete
	case sub == "tag" && onlyFlags("-l", "--list"):
		return ActionView
	case sub == "push" && hasArg("-f", "--force", "--force-with-lease", "-d", "--delete", "--mirror"):
		return ActionDelete
	case sub == "remote" && onlyFlags("-v", "--verbose"):
		return ActionView
	case sub == "config" && len(args) > 0 && slices.Contains([]string{"--get", "--get-all", "--list", "-l"}, args[0]):
		return ActionView
	}
	return ActionEdit
}

var dockerViewSubcommands = []string{
	"ps", "images", "logs", "inspect", "version", "info", "stats", "top", "port", "history",
	"events", "search", "diff",
}

func classifyDocker(parts []string) CommandActionType {
	if len(parts) == 0 {
		return ActionView
	}
	switch parts[0] {
	case "rm", "rmi", "prune":
		return ActionDelete
	case "exec", "attach":
		return ActionOther
	}
	if slices.Contains(dockerViewSubcommands, parts[0]) {
		return ActionView
	}
	// Management commands: docker image prune, docker volume rm, docker compose ps, ...
	if len(parts) > 1 {
		switch parts[1] {
		case "rm", "prune":
			return ActionDelete
		case "ls", "ps", "logs", "inspect", "config", "images", "top":
			return ActionView
		case "exec", "run":
			return ActionOther
		}
	}
	return ActionEdit
}

var kubectlViewSubcommands = []string{
	"get", "describe", "logs", "top", "explain", "version", "api-resources", "api-versions",
	"cluster-info", "diff",
}

func classifyKubectl(parts []string) CommandActionType {
	if len(parts) == 0 {
		return ActionView
	}
	switch sub := parts[0]; {
	case slices.Contains(kubectlViewSubcommands, sub):
		return ActionView
	case sub == "config" && len(parts) > 1 &&
		slices.Contains([]string{"view", "get-contexts", "current-context", "get-clusters"}, parts[1]):
		return ActionView
	case sub == "auth" && len(parts) > 1 && parts[1] == "can-i":
		return ActionView
	case sub == "delete":
		return ActionDelete
	case sub == "exec" || sub == "run" || sub == "attach" || sub == "debug":
		return ActionOther
	}
	return ActionEdit
}

var systemctlViewSubcommands = []string{
	"status", "show", "cat", "list-units", "list-unit-files", "list-timers", "list-sockets",
	"list-dependencies", "is-active", "is-enabled", "is-failed",
}

func classifySystemctl(parts []string) CommandActionType {
	if len(parts) == 0 {
		return ActionView
	}
	switch sub := parts[0]; {
	case slices.Contains(systemctlViewSubcommands, sub):
		return ActionView
	case slices.Contains([]string{"reboot", "poweroff", "halt", "suspend", "hibernate", "kexec", "rescue", "emergency"}, sub):
		return ActionOther
	}
	return ActionEdit
}

// hasOutputRedirection reports whether segment writes to a file through an
// unquoted > or >> (fd duplications like 2>&1 and /dev/null are ignored).
func hasOutputRedirection(segment string) bool {
	rs := []rune(segment)
	inSingle, inDouble := false, false
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case inSingle:
			if r == '\'' {
				inSingle = false
			}
			continue
		case r == '\\':
			i++
			continue
		case r == '\'' && !inDouble:
			inSingle = true
			continue
		case r == '"':
			inDouble = !inDouble
			continue
		case inDouble || r != '>':
			continue
		}

		j := i + 1
		if j < len(rs) && (rs[j] == '>' || rs[j] == '|') {
			j++
		}
		if j < len(rs) && rs[j] == '&' {
			// >&2 duplicates a descriptor; >&file (rare) is not detected
			i = j
			continue
		}
		target := strings.TrimLeft(string(rs[j:]), " \t")
		if strings.HasPrefix(target, "/dev/null") || strings.HasPrefix(target, "/dev/stderr") ||
			strings.HasPrefix(target, "/dev/stdout") {
			i = j
			continue
		}
		return true
	}
	return false
}

func isPackageManager(cmd string) bool {
	packageManagers := []string{
		"apt", "apt-get", "yum", "dnf", "pacman", "brew", "snap",
		"npm", "yarn", "pip", "pip3", "gem", "cargo",
	}
	return slices.Contains(packageManagers, cmd)
}
