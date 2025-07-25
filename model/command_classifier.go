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

// ClassifyCommand analyzes a command and determines its action type
func ClassifyCommand(command string) CommandActionType {
	// Normalize the command
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return ActionOther
	}

	// Split the command to analyze
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return ActionOther
	}

	mainCmd := parts[0]
	
	// Check for shell redirections and pipes
	hasOutputRedirection := false
	hasAppendRedirection := false
	for i, part := range parts {
		if part == ">" && i > 0 {
			hasOutputRedirection = true
		} else if part == ">>" && i > 0 {
			hasAppendRedirection = true
		}
	}

	// Special case: echo with redirection
	if mainCmd == "echo" {
		if hasOutputRedirection || hasAppendRedirection {
			return ActionEdit
		}
		return ActionView
	}

	// Classify based on the main command
	switch mainCmd {
	// View commands
	case "cat", "less", "more", "head", "tail", "grep", "find", "ls", "ll", "la",
		"ps", "top", "htop", "df", "du", "free", "netstat", "ss", "lsof",
		"which", "whereis", "file", "stat", "wc", "sort", "uniq",
		"cut", "paste", "join", "comm", "diff",
		"tree", "pwd", "whoami", "id", "groups", "hostname", "uname",
		"date", "cal", "uptime", "w", "who", "last", "history",
		"printenv", "env", "set", "alias", "type", "command",
		"man", "info", "help", "apropos", "whatis",
		"dig", "nslookup", "host", "ping", "traceroute", "curl", "wget",
		"systemctl", "service", "journalctl", "dmesg",
		"git", "docker", "kubectl":
		// Special handling for some commands that might have subcommands
		if mainCmd == "git" && len(parts) > 1 {
			switch parts[1] {
			case "rm", "clean":
				return ActionDelete
			case "add", "commit", "push", "pull", "merge", "rebase":
				return ActionEdit
			default:
				return ActionView
			}
		}
		if mainCmd == "docker" && len(parts) > 1 {
			switch parts[1] {
			case "rm", "rmi", "prune":
				return ActionDelete
			case "build", "run", "create", "start", "stop", "restart":
				return ActionEdit
			default:
				return ActionView
			}
		}
		if mainCmd == "systemctl" && len(parts) > 1 {
			switch parts[1] {
			case "start", "stop", "restart", "enable", "disable":
				return ActionEdit
			default:
				return ActionView
			}
		}
		return ActionView

	// Edit commands
	case "vim", "vi", "nano", "emacs", "code", "subl", "atom", "gedit", "kate",
		"nvim", "neovim", "ed", "sed", "awk",
		"touch", "mkdir", "cp", "mv", "ln", "chmod", "chown", "chgrp",
		"tar", "zip", "unzip", "gzip", "gunzip", "bzip2", "bunzip2",
		"tee", "dd", "rsync", "scp", "sftp",
		"apt", "apt-get", "yum", "dnf", "pacman", "brew", "snap",
		"npm", "yarn", "pip", "gem", "cargo", "go", "make", "cmake",
		"gcc", "g++", "clang", "python", "ruby", "node", "java", "javac":
		// Check if it's a package manager installing/removing
		if isPackageManager(mainCmd) && len(parts) > 1 {
			switch parts[1] {
			case "remove", "uninstall", "purge", "autoremove":
				return ActionDelete
			default:
				return ActionEdit
			}
		}
		return ActionEdit

	// Delete commands
	case "rm", "rmdir", "unlink", "shred",
		"truncate", "wipefs":
		return ActionDelete

	default:
		// Check for command with path that might be an editor
		if strings.Contains(mainCmd, "/") {
			baseName := mainCmd[strings.LastIndex(mainCmd, "/")+1:]
			return ClassifyCommand(baseName + " " + strings.Join(parts[1:], " "))
		}
		
		// If we have output redirection with unknown command, consider it edit
		if hasOutputRedirection {
			return ActionEdit
		}
		
		return ActionOther
	}
}

func isPackageManager(cmd string) bool {
	packageManagers := []string{
		"apt", "apt-get", "yum", "dnf", "pacman", "brew", "snap",
		"npm", "yarn", "pip", "pip3", "gem", "cargo",
	}
	return slices.Contains(packageManagers, cmd)
}