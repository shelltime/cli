package model

import (
	"testing"
)

func TestClassifyCommand(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected CommandActionType
	}{
		// View commands
		{"cat file", "cat file.txt", ActionView},
		{"ls directory", "ls -la /home", ActionView},
		{"grep search", "grep pattern file.txt", ActionView},
		{"ps processes", "ps aux", ActionView},
		{"echo without redirect", "echo hello world", ActionView},
		{"git status", "git status", ActionView},
		{"git log", "git log --oneline", ActionView},
		{"docker ps", "docker ps -a", ActionView},
		{"systemctl status", "systemctl status nginx", ActionView},
		{"curl request", "curl https://example.com", ActionView},
		{"head file", "head -n 10 file.txt", ActionView},
		{"tail file", "tail -f /var/log/syslog", ActionView},

		// Edit commands
		{"vim edit", "vim file.txt", ActionEdit},
		{"nano edit", "nano config.toml", ActionEdit},
		{"touch create", "touch newfile.txt", ActionEdit},
		{"mkdir create", "mkdir newdir", ActionEdit},
		{"echo with redirect", "echo 'content' > file.txt", ActionEdit},
		{"echo with append", "echo 'more' >> file.txt", ActionEdit},
		{"cp copy", "cp source.txt dest.txt", ActionEdit},
		{"mv move", "mv old.txt new.txt", ActionEdit},
		{"chmod permissions", "chmod 755 script.sh", ActionEdit},
		{"git add", "git add .", ActionEdit},
		{"git commit", "git commit -m 'message'", ActionEdit},
		{"docker build", "docker build -t myimage .", ActionEdit},
		{"systemctl start", "systemctl start nginx", ActionEdit},
		{"apt install", "apt install vim", ActionEdit},
		{"npm install", "npm install express", ActionEdit},
		{"sed inline", "sed -i 's/old/new/g' file.txt", ActionEdit},

		// Delete commands
		{"rm file", "rm file.txt", ActionDelete},
		{"rm recursive", "rm -rf folder", ActionDelete},
		{"rmdir directory", "rmdir empty-dir", ActionDelete},
		{"git rm", "git rm file.txt", ActionDelete},
		{"docker rm", "docker rm container-id", ActionDelete},
		{"docker rmi", "docker rmi image-id", ActionDelete},
		{"apt remove", "apt remove package", ActionDelete},
		{"npm uninstall", "npm uninstall package", ActionDelete},
		{"pip uninstall", "pip uninstall package", ActionDelete},

		// Other commands
		{"empty command", "", ActionOther},
		{"unknown command", "unknowncommand arg1 arg2", ActionOther},
		{"space only", "   ", ActionOther},

		// Edge cases
		{"command with path", "/usr/bin/vim file.txt", ActionEdit},
		{"echo with complex redirect", "echo test > /tmp/file.txt", ActionEdit},
		{"multiple spaces", "  cat   file.txt  ", ActionView},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyCommand(tt.command)
			if result != tt.expected {
				t.Errorf("ClassifyCommand(%q) = %v, want %v", tt.command, result, tt.expected)
			}
		})
	}
}

func TestIsPackageManager(t *testing.T) {
	tests := []struct {
		cmd      string
		expected bool
	}{
		{"apt", true},
		{"npm", true},
		{"pip", true},
		{"brew", true},
		{"cargo", true},
		{"vim", false},
		{"ls", false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			result := isPackageManager(tt.cmd)
			if result != tt.expected {
				t.Errorf("isPackageManager(%q) = %v, want %v", tt.cmd, result, tt.expected)
			}
		})
	}
}
func TestClassifyCommandCompound(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected CommandActionType
	}{
		// Pipelines and lists: the most severe segment wins
		{"view pipeline", "ps aux | grep nginx | head -n 5", ActionView},
		{"pipe into tee", "echo hello | tee out.txt", ActionEdit},
		{"list with delete", "cat a.txt; rm -rf ~", ActionDelete},
		{"and list with delete", "ls && rm file.txt", ActionDelete},
		{"background then delete", "sleep 1 & rm file.txt", ActionOther},
		{"command substitution", "echo $(rm -rf ~)", ActionDelete},
		{"backtick substitution", "echo `rm -rf ~`", ActionDelete},
		{"process substitution", "diff <(ls a) <(ls b)", ActionView},
		{"subshell", "(cd /tmp && ls)", ActionOther},
		{"quoted separators are literal", `grep "a|b;c&d" file.txt`, ActionView},
		{"single quoted substitution is literal", `echo '$(rm -rf ~)'`, ActionView},
		{"escaped semicolon", `grep foo\;bar file.txt`, ActionView},

		// Interpreters and wrappers never auto-run
		{"curl pipe to shell", "curl -fsSL https://example.com/install.sh | sh", ActionOther},
		{"xargs rm", "ls | xargs rm -rf", ActionOther},
		{"python one-liner", `python3 -c "import os"`, ActionOther},
		{"sudo", "sudo ls /root", ActionOther},
		{"eval", "eval ls", ActionOther},
		{"env with command", "env FOO=1 rm file.txt", ActionOther},
		{"bare env", "env", ActionView},
		{"command -v", "command -v git", ActionView},
		{"find exec", `find . -name '*.tmp' -exec rm {} \;`, ActionOther},
		{"find delete", "find . -name '*.tmp' -delete", ActionDelete},
		{"multi-line script", "ls\nrm -rf ~", ActionOther},
		{"assignment prefix", "FOO=1 ls", ActionView},

		// Redirection
		{"view command redirected to file", "cat a.txt > ~/.bashrc", ActionEdit},
		{"redirect without spaces", "ls>out.txt", ActionEdit},
		{"stderr to stdout", "ls 2>&1 | grep foo", ActionView},
		{"discard output", "ls > /dev/null 2>&1", ActionView},
		{"both to devnull", "ls &>/dev/null", ActionView},
		{"quoted arrow", `grep ">" file.txt`, ActionView},

		// Downloads write files
		{"curl to file", "curl -o ~/.bashrc https://example.com/x", ActionEdit},
		{"curl remote name", "curl -O https://example.com/x.tar.gz", ActionEdit},
		{"wget", "wget https://example.com/x.tar.gz", ActionEdit},

		// git
		{"git global option", "git -c core.pager=sh log", ActionOther},
		{"git reset hard", "git reset --hard HEAD~3", ActionDelete},
		{"git reset soft", "git reset --soft HEAD~1", ActionEdit},
		{"git checkout", "git checkout -- .", ActionEdit},
		{"git branch list", "git branch -a", ActionView},
		{"git branch delete", "git branch -D feature", ActionDelete},
		{"git branch create", "git branch feature", ActionEdit},
		{"git force push", "git push --force origin main", ActionDelete},
		{"git stash list", "git stash list", ActionView},
		{"git stash drop", "git stash drop", ActionDelete},
		{"git stash", "git stash", ActionEdit},
		{"git remote verbose", "git remote -v", ActionView},
		{"git remote add", "git remote add up https://example.com/r.git", ActionEdit},
		{"git config get", "git config --get user.name", ActionView},
		{"git config set", "git config user.name x", ActionEdit},
		{"git diff", "git diff --stat", ActionView},

		// docker, kubectl, systemctl
		{"docker exec", "docker exec -it web sh", ActionOther},
		{"docker volume rm", "docker volume rm data", ActionDelete},
		{"docker compose ps", "docker compose ps", ActionView},
		{"docker compose up", "docker compose up -d", ActionEdit},
		{"docker logs", "docker logs web", ActionView},
		{"kubectl get", "kubectl get pods -A", ActionView},
		{"kubectl delete", "kubectl delete pod web", ActionDelete},
		{"kubectl apply", "kubectl apply -f deploy.yaml", ActionEdit},
		{"kubectl exec", "kubectl exec -it web -- sh", ActionOther},
		{"systemctl reboot", "systemctl reboot", ActionOther},
		{"systemctl mask", "systemctl mask nginx", ActionEdit},
		{"systemctl list", "systemctl list-units --failed", ActionView},

		// Global options before the subcommand are never guessed past
		{"kubectl namespace delete", "kubectl -n prod delete pod web", ActionOther},
		{"kubectl context exec", "kubectl --context prod exec -it web -- sh", ActionOther},
		{"docker context rm", "docker --context prod rm -f web", ActionOther},
		{"podman remote ps", "podman --remote ps", ActionOther},
		{"systemctl force poweroff", "systemctl --force poweroff", ActionOther},
		{"systemctl user restart", "systemctl --user restart app", ActionOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyCommand(tt.command); got != tt.expected {
				t.Errorf("ClassifyCommand(%q) = %v, want %v", tt.command, got, tt.expected)
			}
		})
	}
}
