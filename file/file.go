package file

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"

	"path"
	"text/template"

	"github.com/docker/docker/pkg/term"
	log "github.com/sirupsen/logrus"
)

var (
	re           = regexp.MustCompile("[^a-zA-Z0-9]")
	ErrSkipBuild = errors.New("skip build")
)

type Dapperfile struct {
	File        string
	Mode        string
	docker      string
	env         Context
	Socket      bool
	NoOut       bool
	Args        []string
	From        string
	Quiet       bool
	hostArch    string
	Keep        bool
	NoContext   bool
	MapUser     bool
	PushTo      string
	PullFrom    string
	Variant     string
	MountSuffix string
}

func Lookup(file string) (*Dapperfile, error) {
	if _, err := os.Stat(file); err != nil {
		return nil, err
	}

	d := &Dapperfile{
		File: file,
	}

	return d, d.init()
}

func (d *Dapperfile) init() error {
	docker, err := exec.LookPath("docker")
	if err != nil {
		return err
	}
	d.docker = docker
	if d.Args, err = d.argsFromEnv(d.File); err != nil {
		return err
	}
	if d.hostArch == "" {
		d.hostArch = d.findHostArch()
	}
	return nil
}

func (d *Dapperfile) argsFromEnv(dockerfile string) ([]string, error) {
	file, err := os.Open(dockerfile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	r := []string{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		fields := strings.Fields(line)
		if len(fields) <= 1 {
			continue
		}

		command := fields[0]
		if command != "ARG" {
			continue
		}

		key := strings.Split(fields[1], "=")[0]
		value := os.Getenv(key)

		if key == "DAPPER_HOST_ARCH" && value == "" {
			value = d.findHostArch()
		}

		if key == "DAPPER_HOST_ARCH" {
			d.hostArch = value
		}

		if value != "" {
			r = append(r, fmt.Sprintf("%s=%s", key, value))
		}
	}

	return r, nil
}

func (d *Dapperfile) RemoteImageNameWithTag(arg string) (string, error) {
	tmpl, err := template.New("remote-tag").Parse(arg)
	if err != nil {
		panic(err)
	}

	var remoteTag bytes.Buffer
	err = tmpl.Execute(&remoteTag, d)
	if err != nil {
		panic(err)
	}

	name := remoteTag.String()

	/*
		go templates are powerful but hard to type. Let's help users with some magic:

		- if no ":" is in the name AND name ends with /
			e.g. "registry.example.com/ci/"
			add name and tag automatically:
			"registry.example.com/ci/myimage:master"
	*/

	if !strings.Contains(name, ":") && strings.HasSuffix(name, "/") {
		log.Debugf("remote specification will be auto-completed")
		name = name + d.ImageNameWithTag()
	}

	log.Debugf("image/tag map: '%s' <=> '%s'", d.ImageNameWithTag(), name)

	return name, nil
}

func (d *Dapperfile) PushImage() error {
	remoteName, err := d.RemoteImageNameWithTag(d.PushTo)
	if err != nil {
		return err
	}

	err = d.exec("tag", d.ImageNameWithTag(), remoteName)
	if err != nil {
		return err
	}
	err = d.exec("push", remoteName)
	return err
}

func (d *Dapperfile) PullImage() error {
	remoteName, err := d.RemoteImageNameWithTag(d.PullFrom)
	if err != nil {
		return err
	}

	err = d.exec("pull", remoteName)
	if err != nil {
		log.Warnf("Could not pull %s remoteName: %v", remoteName, err)
		return nil
	}

	err = d.exec("tag", remoteName, d.ImageNameWithTag())
	if err != nil {
		return err
	}
	return err
}

func (d *Dapperfile) Run(commandArgs []string) error {
	imageNameWithTag, err := d.build()
	if err != nil {
		return err
	}

	log.Debugf("Running build in %s", imageNameWithTag)
	name, args, err := d.runArgs(imageNameWithTag, "", commandArgs)
	if err != nil {
		return err
	}

	defer func() {
		if d.Keep {
			log.Infof("Keeping build container %s", name)
		} else {
			log.Debugf("Deleting temp container %s", name)
			d.execWithOutput("rm", "-fv", name)
		}
	}()

	if err := d.run(args...); err != nil {
		return err
	}

	source := d.env.Source()
	output := d.env.Output()
	if !d.IsBind() && !d.NoOut {
		for _, i := range output {
			p := i
			if !strings.HasPrefix(p, "/") {
				p = path.Join(source, i)
			}
			targetDir := path.Dir(i)
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				return err
			}
			log.Infof("docker cp %s %s", p, targetDir)
			if err := d.exec("cp", name+":"+p, targetDir); err != nil {
				log.Debugf("Error copying back '%s': %s", i, err)
			}
		}
	}

	return nil
}

func (d *Dapperfile) Shell(commandArgs []string) error {
	imageNameWithTag, err := d.build()
	if err != nil {
		return err
	}

	log.Debugf("Running shell in %s", imageNameWithTag)
	_, args, err := d.runArgs(imageNameWithTag, d.env.Shell(), nil)
	args = append([]string{"--rm"}, args...)
	if err != nil {
		return err
	}

	return d.runExec(args...)
}

func (d *Dapperfile) runArgs(imageNameWithTag, shell string, commandArgs []string) (string, []string, error) {
	name := fmt.Sprintf("%s-%s", strings.Split(imageNameWithTag, ":")[0], randString())

	args := []string{"-i", "--name", name}

	if term.IsTerminal(0) {
		args = append(args, "-t")
	}

	if d.env.Socket() || d.Socket {
		args = append(args, "-v", fmt.Sprintf("%s:/var/run/docker.sock", d.env.HostSocket()))
	}

	if d.IsBind() {
		wd, err := os.Getwd()
		if err == nil {
			suffix := ""
			if d.env.MountSuffix(d.MountSuffix) != "" {
				suffix = ":" + d.env.MountSuffix(d.MountSuffix)
			}
			args = append(args, "-v", fmt.Sprintf("%s:%s%s", fmt.Sprintf("%s/%s", wd, d.env.Cp()), d.env.Source(), suffix))
		}
	}

	args = append(args, "-e", fmt.Sprintf("DAPPER_UID=%d", os.Getuid()))
	args = append(args, "-e", fmt.Sprintf("DAPPER_GID=%d", os.Getgid()))
	args = append(args, "-e", "DAPPER=1")

	for _, env := range d.env.Env() {
		log.Debugf("mapping env %s", env)
		args = append(args, "-e", env)
	}

	volumes, err := d.env.Volumes()
	if err != nil {
		return "", nil, err
	}

	for _, vol := range volumes {
		log.Debugf("mapping volume %s", vol)
		args = append(args, "-v", vol)
	}

	if shell != "" {
		args = append(args, "--entrypoint", shell)
		args = append(args, "-e", "TERM")
	}

	if d.MapUser {
		args = append(args, "-u", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()))
		args = append(args, "-v", "/etc/passwd:/etc/passwd:ro")
		args = append(args, "-v", "/etc/group:/etc/group:ro")
	}

	args = append(args, d.env.RunArgs()...)
	args = append(args, imageNameWithTag)

	if shell != "" && len(commandArgs) == 0 {
		args = append(args, "-")
	} else {
		args = append(args, commandArgs...)
	}

	return name, args, nil
}

func (d *Dapperfile) prebuild() error {
	f, err := os.Open(d.File)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	target := ""

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "FROM ") {
			parts := strings.Fields(line)
			if len(parts) <= 1 {
				return nil
			}

			target = strings.TrimSpace(parts[1])
			continue
		} else if target == "" || !strings.HasPrefix(line, "# FROM ") {
			continue
		}

		baseImage, ok := toMap(line)[d.hostArch]
		if !ok {
			return nil
		}

		if baseImage == "skip" {
			return ErrSkipBuild
		}

		_, err := exec.Command(d.docker, "inspect", baseImage).CombinedOutput()
		if err != nil {
			if err := d.exec("pull", baseImage); err != nil {
				return err
			}
		}

		log.Debugf("Running tag with %s %s", baseImage, target)
		return d.exec("tag", baseImage, target)
	}

	return scanner.Err()
}

func (d *Dapperfile) findHostArch() string {
	output, err := d.execWithOutput("version", "-f", "{{.Server.Arch}}")
	if err != nil {
		return runtime.GOARCH
	}
	return strings.TrimSpace(string(output))
}

func (d *Dapperfile) Build(args []string) error {
	if err := d.prebuild(); err != nil {
		return err
	}

	buildArgs := []string{"build"}

	for _, v := range d.Args {
		buildArgs = append(buildArgs, "--build-arg", v)
	}
	buildArgs = append(buildArgs, args...)

	if d.NoContext {
		buildArgs = append(buildArgs, "-")

		stdinFile, err := os.Open(d.File)
		if err != nil {
			return err
		}
		defer stdinFile.Close()

		return d.execWithStdin(stdinFile, buildArgs...)
	}

	buildArgs = append(buildArgs, "-f", d.File)

	// Always attempt to pull a newer version of the base image
	buildArgs = append(buildArgs, "--pull")

	return d.exec(buildArgs...)
}

func (d *Dapperfile) build() (string, error) {
	if err := d.prebuild(); err != nil {
		return "", err
	}

	imageNameWithTag := d.ImageNameWithTag()

	log.Debugf("Building %s using %s", imageNameWithTag, d.File)
	buildArgs := []string{"build", "-t", imageNameWithTag}

	if d.Quiet {
		buildArgs = append(buildArgs, "-q")
	}

	for _, v := range d.Args {
		buildArgs = append(buildArgs, "--build-arg", v)
	}

	// Always attempt to pull a newer version of the base image
	buildArgs = append(buildArgs, "--pull")

	if d.NoContext {
		buildArgs = append(buildArgs, "-")

		stdinFile, err := os.Open(d.File)
		if err != nil {
			return "", err
		}
		defer stdinFile.Close()

		if err := d.execWithStdin(stdinFile, buildArgs...); err != nil {
			return "", err
		}
	} else {
		buildArgs = append(buildArgs, "-f", d.File)
		buildArgs = append(buildArgs, ".")

		if err := d.exec(buildArgs...); err != nil {
			return "", err
		}
	}

	if err := d.readEnv(imageNameWithTag); err != nil {
		return "", err
	}

	if !d.IsBind() {
		text := fmt.Sprintf("FROM %s\nCOPY %s %s", imageNameWithTag, d.env.Cp(), d.env.Source())
		if err := d.buildWithContent(imageNameWithTag, text); err != nil {
			return "", err
		}
	}

	return imageNameWithTag, nil
}

func (d *Dapperfile) buildWithContent(tag, content string) error {
	tempfile, err := ioutil.TempFile(".", d.File)
	if err != nil {
		return err
	}

	log.Debugf("Created tempfile %s", tempfile.Name())
	defer func() {
		log.Debugf("Deleting tempfile %s", tempfile.Name())
		if err := os.Remove(tempfile.Name()); err != nil {
			log.Errorf("Failed to delete tempfile %s: %v", tempfile.Name(), err)
		}
	}()

	ioutil.WriteFile(tempfile.Name(), []byte(content), 0600)

	return d.exec("build", "-t", tag, "-f", tempfile.Name(), ".")
}

func (d *Dapperfile) readEnv(tag string) error {
	var envList []string

	args := []string{"inspect", "-f", "{{json .ContainerConfig.Env}}", tag}

	cmd := exec.Command(d.docker, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to run docker %v: %v", args, err)
		return err
	}

	if err := json.Unmarshal(output, &envList); err != nil {
		return err
	}

	d.env = map[string]string{}

	for _, item := range envList {
		parts := strings.SplitN(item, "=", 2)
		k, v := parts[0], parts[1]
		log.Debugf("Reading Env: %s=%s", k, v)
		d.env[k] = v
	}

	log.Debugf("Source: %s", d.env.Source())
	log.Debugf("Cp: %s", d.env.Cp())
	log.Debugf("Socket: %t", d.env.Socket())
	log.Debugf("Mode: %s", d.env.Mode(d.Mode))
	log.Debugf("Env: %v", d.env.Env())
	log.Debugf("Output: %v", d.env.Output())

	volumes, _ := d.env.Volumes()
	log.Debugf("Volumes: %v", volumes)

	return nil
}

func (d *Dapperfile) ImageName() string {
	cwd, err := os.Getwd()
	if err == nil {
		cwd = filepath.Base(cwd)
	} else {
		cwd = "dapper-unknown"
	}

	if d.Variant != "" {
		cwd = fmt.Sprintf("%s-%s", cwd, d.Variant)
	}

	// repository name must be lowercase
	cwd = strings.ToLower(cwd)
	// repository must not include @ (e.g. Jenkins workspace)
	// re-using re definition as safeguard
	cwd = re.ReplaceAllLiteralString(cwd, "-")

	return cwd
}

func (d *Dapperfile) Tag() string {
	output, _ := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	tag := strings.TrimSpace(string(output))
	if tag == "" {
		tag = randString()
	}
	tag = re.ReplaceAllLiteralString(tag, "-")

	return tag
}

func (d *Dapperfile) ImageNameWithTag() string {
	return fmt.Sprintf("%s:%s", d.ImageName(), d.Tag())
}

func (d *Dapperfile) run(args ...string) error {
	return d.exec(append([]string{"run"}, args...)...)
}

func (d *Dapperfile) exec(args ...string) error {
	log.Debugf("Running %s %v", d.docker, args)
	cmd := exec.Command(d.docker, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	err := cmd.Run()
	if err != nil {
		log.Debugf("Failed running %s %v: %v", d.docker, args, err)
	}
	return err
}

func (d *Dapperfile) execWithStdin(stdin *os.File, args ...string) error {
	log.Debugf("Running %s %v", d.docker, args)
	cmd := exec.Command(d.docker, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = stdin
	err := cmd.Run()
	if err != nil {
		log.Debugf("Failed running %s %v: %v", d.docker, args, err)
	}
	return err
}

func (d *Dapperfile) runExec(args ...string) error {
	log.Debugf("Exec %s run %v", d.docker, args)
	return syscall.Exec(d.docker, append([]string{"docker", "run"}, args...), os.Environ())
}

func (d *Dapperfile) execWithOutput(args ...string) ([]byte, error) {
	cmd := exec.Command(d.docker, args...)
	return cmd.CombinedOutput()
}

func (d *Dapperfile) IsBind() bool {
	return d.env.Mode(d.Mode) == "bind"
}
