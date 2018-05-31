# PSPDFKit's Dapper fork
This is PSPDFKit's fork of [Rancher's dapper](https://github.com/rancher/dapper), a tool to wrap any existing build tool in an consistent environment.

Our use case is to run existing test jobs within a defined environment (= Docker container) and to keep things as simple as possible and as clear as possbile. We don't want extra steps to build and release base images for various testing, we don't want to have logic defined in the CI system. As much as possible should stay inside the version control system.

Caveat: Dapper is not intended to be used to build "production images". Support for pulling and pushing images to a docker registry was added to provide some kind of distributed build cache, only.
Dockerfiles for use with dapper usually contain special instructions based on `ENV` statements (environment variables) and should also be named with a `.dapper` suffic.

Please see the example section at the end of this README for practical use cases and useful combination of switches.

## Installation

In general binary releases are available at [https://github.com/pspdfkit-ops/dapper/releases](https://github.com/pspdfkit-ops/dapper/releases).


#### macOS using Homebrew

```Shell
brew tap pspdfkit-ops/repo
brew install dapper
```

## Usage

1. create a `Dockerfile.dapper` to define a docker image
2. `dapper -- /bin/sh -c 'echo "hello from the inside"'`

In a real-world use case you would specify your build/test environment in `Dockerfile.dapper` and call the existing jobs using dapper. For example let's say you have a script called `bin/ci-lint` which your CI system usually executes. After adding the `Dockerfile.dapper` you can just call `dapper -- bin/ci-lint`. Congrats, your job now runs inside a docker container. No custom build/test node configuration needed anymore.

### Actions, Flags, Configuration and Defaults

Dapper can be called with various cli flags, you can set values using environment variables (`DAPPER_<setting_in_uppercase>`, i.ex. `DAPPER_DEBUG=1`), use a configuration file (`dapper{.yaml|.json|.toml}`) or everything together. CLI flags have the highest priority.


```
Flags:
      --build                      Perform a build
  -s, --shell                      Launch a shell
      --config string              config file (default is $PWD/dapper.yaml)
  -d, --debug                      Print debugging
  -C, --directory string           The directory in which to run, --file is relative to this (default ".")
  -f, --file string                Dockerfile to build from (default "Dockerfile.dapper")
      --generate-bash-completion   Generates Bash completion script to Stdout
  -h, --help                       help for dapper
      --keep                       Don't remove the container that was used to build
  -u, --map-user                   Map UID/GID from dapper process to docker run
  -m, --mode string                Execution mode for Dapper bind/cp/auto (default "auto")
  -X, --no-context                 send Dockerfile via stdin to docker build command
  -O, --no-out                     Do not copy the output back (in --mode cp)
      --pull-from string           Pulls a build image to the location
      --push-to string             Publishes a build image to the location
  -q, --quiet                      Make Docker build quieter
  -k, --socket                     Bind in the Docker socket
  -v, --version                    Show version
```


#### Dockerfile.dapper

```shell
  Dockerfile variables

  DAPPER_SOURCE          The destination directory in the container to bind/copy the source
  DAPPER_CP              The location in the host to find the source
  DAPPER_OUTPUT          The files you want copied to the host in CP mode
  DAPPER_DOCKER_SOCKET   Whether the Docker socket should be bound in
  DAPPER_RUN_ARGS        Args to add to the docker run command when building
  DAPPER_ENV             Env vars that should be copied into the build
  DAPPER_VOLUMES         Volumes that should be mounted on docker run.
```

## Features


#### Magic Environment Variables in Dockerfile.dapper

tbd

#### Variants

Let's say you have a huge mono repo with lots of different projects. Some projects can share the same image, other ones need a custom one. Just create multiple files.

Example layout:

```
monorepo
├── Dockerfile.ruby.dapper
├── Dockerfile.e2e.dapper
└── Dockerfile.cpp.dapper
```

| Dapperfile | Variant | image name |
|-|-|-|
|Dockerfile.dapper|(none)|monorepo:$branch|
|Dockerfile.ruby.dapper|ruby|monorepo-ruby:$branch|
|Dockerfile.e2e.dapper|e2e|monorepo-e2e:$branch|
|Dockerfile.cpp.dapper|cpp|monorepo-cpp:$branch|



#### User mapping (UID/GID) and context-free images

There are several ways to get your data into a container. One is to ADD the files at image build time, by building an intermediary image (`cp` mode), and using volume mount (`bind` mode) it when the container starts. The latter is the preferred workflow as it keeps your images clean and "runtime only".

Even when you don't ADD your code in the `Dockerfile.dapper`, docker cli will send everything to
dockerd to build the image (build context) which sucks in case of huge monorepos.

`dapper -X` will create the image outside of the current context and bind/volume-mount the current directory. This can be tricky because of user permissions and different user ids. Therefore by default the container will also mount `/etc/passwd` and `/etc/group` from the host to match the UID. This is handy if your command creates a new file, e.g. code coverage or a binary artifact. This files can then be picked up e.g. by a Jenkins plugin.


#### Pull/Push Images

Let's say you have serveral build nodes. You don't want to re-build the images on each node. By using the `--pull-from` and `--push-to` options, images will be pulled and/or pushed from/to a Docker repository. Docker's own image management also act's as a local cache.

e.g.

```shell
  dapper \
    --pull-from registry.docker.example.com/ci/project:latest \
    --push-to   registry.docker.example.com/ci/project:latest
```

or use the fully automagic mode:

```shell
  dapper \
    --pull-from registry.example.com/ci/ \
    --push-to   registry.example.com/ci/
```
(hint: last character must be a slash, `:` must not appear)
Dapper will then automatically create the right name.

Example:

| project directory | normalized name (auto) | branch (auto) | Filename |  variant (auto) | name and tag (auto) | full name (auto)|
|-|-|-|-|-|-|-|-|-|
| example | example | staging | Dockerfile.e2e.dapper | e2e | example-e2e:staging | registry.example.com/ci/example-e2e:staging |
| Project@0 | project | user/branch | Dockerfile.dapper | (none) | project:user-branch | registry.example.com/ci/project:user-branch |
| Project@1 | project |user/branch | Dockerfile.cpp.dapper | cpp| project:user-branch | registry.example.com/ci/project-cpp:user-branch |


##### Go templating

You can also use the [Go template syntax](https://golang.org/pkg/text/template) to construct the target and even specify everything in a `docker.toml` file:

```toml
no-context = true
map-user = true
pull-from = "registry.example.com/ci/project-{{ .Variant }}:{{ .Tag }}"
push-to = "registry.example.com/ci/project-{{ .Variant }}:{{ .Tag }}"
```



## Examples and How-to


see `examples` directory (WIP).


#### Alternatives

- Test Kitchen + Kitchen-Docker (Ruby)
- various solutions tied to a specific CI system (e.g. Gitlab CI, drone, travis, Jenkins)