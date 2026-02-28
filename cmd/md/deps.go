package main

import (
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"runtime"
)

// fileSystem abstracts os-level file operations.
type fileSystem interface {
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm fs.FileMode) error
	MkdirAll(path string, perm fs.FileMode) error
}

// httpClient abstracts HTTP request execution (satisfied by *http.Client).
type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// shellRunner abstracts running a shell command that reads from an input
// file and writes to an output file. The runner manages temp file lifecycle.
type shellRunner interface {
	Run(command string, inputData []byte, inputEnvVar, outputEnvVar string, stderr io.Writer) ([]byte, error)
}

// osFileSystem delegates to the os package.
type osFileSystem struct{}

func (osFileSystem) ReadFile(name string) ([]byte, error)                  { return os.ReadFile(name) }
func (osFileSystem) WriteFile(name string, data []byte, perm fs.FileMode) error { return os.WriteFile(name, data, perm) }
func (osFileSystem) MkdirAll(path string, perm fs.FileMode) error         { return os.MkdirAll(path, perm) }

// osShellRunner creates temp files, runs a command via the system shell,
// reads the output, and cleans up.
type osShellRunner struct{}

func (osShellRunner) Run(command string, inputData []byte, inputEnvVar, outputEnvVar string, stderr io.Writer) ([]byte, error) {
	inputFile, err := os.CreateTemp("", "md-input-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(inputFile.Name())

	if _, err := inputFile.Write(inputData); err != nil {
		inputFile.Close()
		return nil, err
	}
	inputFile.Close()

	outputFile, err := os.CreateTemp("", "md-output-*")
	if err != nil {
		return nil, err
	}
	outputFile.Close()
	defer os.Remove(outputFile.Name())

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	cmd.Env = append(os.Environ(),
		inputEnvVar+"="+inputFile.Name(),
		outputEnvVar+"="+outputFile.Name(),
	)
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return os.ReadFile(outputFile.Name())
}
