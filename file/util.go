package file

import (
	"math/rand"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func init() {
	rand.Seed(time.Now().UnixNano())
}

func randString() string {
	b := make([]byte, 7)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func toMap(str string) map[string]string {
	kv := map[string]string{}

	for _, part := range strings.Fields(str) {
		kvs := strings.SplitN(part, "=", 2)
		if len(kvs) != 2 {
			continue
		}
		kv[kvs[0]] = kvs[1]
	}

	return kv
}

func ExtractErrorCode(err error) int {
	exitCode := 1
	if err != nil {
		// I guess syscall things wont work on windows
		if runtime.GOOS == "windows" {
			return exitCode
		}

		if exitError, ok := err.(*exec.ExitError); ok {
			ws := exitError.Sys().(syscall.WaitStatus)
			exitCode = ws.ExitStatus()
		}
	}

	return exitCode
}

func ExtractVariantFromFilename(filename string) string {
	/*
		Convention:
		- Dockerfile.dapper
		- Dockerfile.variant.dapper
	*/
	variant := ""
	parts := strings.Split(filename, ".")

	if len(parts) == 3 {
		variant = parts[1]
	}

	return variant
}
