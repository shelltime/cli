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