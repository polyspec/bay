# bay

`bay`은 Apple `container`와 Docker 위에서 Compose 프로젝트를 실행하고,
각 서비스를 로컬 도메인 이름으로 HTTPS 제공한다. 서비스 포트를 게시하지 않는다.
모든 서비스 컨테이너가 이 이름들의 인증서를 신뢰하므로 서비스끼리도 서로의 이름에
HTTPS로 접속한다.

기계끼리 서로를 피어로 승인하고, 상호 인증 링크로 서로의 도메인에 접속한다.
Docker를 돌리는 싱글보드 컴퓨터 같은 Linux 기계도 같은 방식으로 설정하며, 자신을
거쳐 오는 기계에서만 연결을 받도록 제한할 수 있다.

한 기계가 두 엔진을 동시에 실행할 수 있다. 엔진마다 자기 프록시를 실행하고 자기
컨테이너를 제공한다. 없는 엔진이나 데몬이 응답하지 않는 엔진은 컨테이너를
제공하지 않으며 오류가 아니다.

프로젝트는 Compose 파일이다. 추가 설정이 없는 서비스는 `<서비스>.test`로
제공된다.

## 시작

```sh
go install github.com/polyspec/bay/cmd/bay@latest
go install github.com/polyspec/bay/cmd/bayd@latest
bay install -dry-run   # 설정이 바꿀 것
bay install            # 머신당 1회. 암호를 묻는다
bay check              # 설정의 각 부분, ok 또는 missing
cd ~/work/your-project
bay up
```

`install`은 빠진 것만 바꾸므로 업데이트 후나 `check`가 빠진 부분을 알릴 때 다시
실행한다. Linux에서는 root로 실행하며, `bay install -only-from 192.168.2.1`은
그 호스트에서만 연결을 받게 한다. [피어](docs/spec/peers.ko.md) 참고.

메뉴바 앱까지 쓰려면 저장소를 클론해 `make`와 `sudo make install`을 실행한다.
[설치](docs/operations/install.ko.md) 참고.

### 고정 release (Linux, CI)

release마다 macOS와 Linux의 arm64, amd64용 `bay`와 `bayd`, 그리고 `SHA256SUMS`가
있다. release나 `make`로 만든 빌드에는 컨테이너 자신의 저장소에 이 기계의
인증기관을 넣는 bayinit이 들어 있다. `go install`로 만든 빌드에는 없고, `bay
check`가 그렇다고 알린다. Linux에서는 설정과 명령을 root로 실행하므로 CI 작업은
`sudo`로 실행한다:

```sh
VERSION=v0.0.1
ARCH=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')
NAME=bay_${VERSION}_linux_${ARCH}
curl -fsSLO https://github.com/polyspec/bay/releases/download/$VERSION/$NAME.tar.gz
curl -fsSLO https://github.com/polyspec/bay/releases/download/$VERSION/SHA256SUMS
grep " $NAME.tar.gz\$" SHA256SUMS | sha256sum -c -
tar -xzf $NAME.tar.gz
sudo install -m 755 $NAME/bay $NAME/bayd /usr/local/bin/
sudo bay install            # root라 아무것도 묻지 않는다
sudo bay check -scope run   # 실행에 필요한 부분이 빠지면 exit 1
```

## 사용법은 명령에 있다

```sh
bay brief           # 전체 사용 계약. 프로그램용은 --json
bay schema          # Compose 파일 계약. 프로그램용은 --json
bay help up         # 명령 하나가 바꾸는 것
bay status --json   # 현재 머신 상태
```

바이너리를 설치한 프로젝트는 이 저장소를 받지 않으므로 명령이 사용법을 담는다.

## 에이전트

에이전트는 명령이 출력하는 것과 같은 계약에서 도구를 배우고, 그 내용은 설치된
바이너리를 따라간다. `install`은 `~/.claude/skills/bay/`과 `~/.agents/skills/bay/` 중
디렉터리가 있는 곳에 skill을 쓰고, `claude` 명령이 있으면 Claude Code 사용자 설정에
`bay mcp`를 등록한다. `make install`, `up`, 그리고 세션마다 시작되는 MCP 서버가 skill이
바이너리와 다르면 다시 쓰고, `check`는 없거나 낡은 skill과 등록을 알린다. MCP 도구 설명은
실행 중인 바이너리가 직접 내놓으며 접근 수준에 따라 읽기 전용·멱등·파괴적으로 표시되고,
`brief`, `schema`, `status`는 리소스로 제공된다. 사람에게 묻는 명령은 에이전트에게서 답을
받지 못하므로 에이전트가 사용자에게 실행을 요청한다. 아래 섹션은 코드에서 생성된다.

<!-- generated: bay brief; run "make docs-generate" -->

### What it is for

- **Run a Compose project as its file says, on Apple container or Docker.** A key the Compose specification defines behaves as it does there, or is rejected by name; nothing is silently ignored.
- **Give every service a name, a certificate and trust.** <project>-<service>.bay.test, known before up from the file alone; a certificate for that name from this machine's authority; containers that trust the authority.
- **Serve the domains a project declares over HTTPS.** One proxy per machine, routes from container labels, certificates issued and renewed by the tool.
- **Reach the names of approved peer machines.** Over the mutually authenticated peer link, and only to machines approved by fingerprint.
- **Be a dependable step for a person, a script or an agent.** Exit status that says whether the request was done, JSON output where a program reads the result, nothing left behind by an interrupted command, and the same result when run twice.
- **Keep a Linux machine reachable while projects use it.** The containers bay runs share a slice that slows them past 70% of memory and holds them under 80% with at most 10% in swap, and the kernel keeps a reserve for the network driver, so a loaded machine is slow but answers; energy-efficient ethernet stays off on its uplink, whose wake from low-power idle dropped the link.
- **Apply the machine setup only on request.** install applies what is missing; every other command reads the setup and names install when a part is missing.

### What it does not do

| Not done | Why | Instead |
| --- | --- | --- |
| Running or stopping a project on another machine. | A project's files, volumes, state and setup are on the machine that runs it; a remote up would act on paths and setup it cannot see. | Run the command on the machine that holds the project, for example over ssh: ssh host bay -f /path/compose.yaml up. |
| Publishing a service port or a Unix socket on the host. | Every service is reached by name, so two projects never compete for a host port and a reachable service is always a named, certified one. | Connect to <project>-<service>.bay.test:<port>; a socket in a bind-mounted directory is plain Compose and works where the engine shares the host's kernel. |
| Waiting without a bound. | A wait with no limit hides a service that never starts; every wait states its bound before it begins. | Give a slow service a longer healthcheck start_period or interval. |
| Deciding a caller's policy: retries, time limits, which images to fetch. | These belong to the caller's rules; the tool states what it does and offers the switches. | bay pull, up -no-pull, healthcheck keys, exit status. |
| Changing the machine setup from a project command. | A setup change asks for administrator rights and affects every project; it happens when a person asks for it. | bay install -dry-run, then bay install. |

```sh
bay install                               # also writes the skill below
bay skill install                         # ~/.claude/skills and ~/.agents/skills
bay mcp install                                    # registers it in Claude Code; install does too
bay brief --json                          # the contract, schemaVersion 1
```

| Command | Summary | Access | Asks a person for |
| --- | --- | --- | --- |
| `bay up [-no-pull] [--json]` | Start this project's services and register their routes. | change | the administrator password, in a system dialog or on the terminal, when a step needs root |
| `bay down` | Remove this project's services and withdraw its routes. | remove | nothing |
| `bay start [service...]` | Start services. With no name, starts every service in the project. | change | nothing |
| `bay stop [service...]` | Stop services. The containers remain and can be started again. | change | nothing |
| `bay restart [service...]` | Stop then start services. | change | nothing |
| `bay logs [-f] [-n N] <service>` | Print one service's output. | read | nothing |
| `bay plan [--json]` | Print each service's name, port, domains and certificate paths as up will create them. | read | nothing |
| `bay exec [-u user] [-w dir] [-e KEY=VALUE] <service> <command...>` | Run a command in a service's running container and exit with its status. | change | nothing |
| `bay pull [service...]` | Fetch the images of this project's services from their registries. | change | nothing |
| `bay images [--json] [service...]` | Report whether the local store holds each service's image. Fetches nothing. | read | nothing |
| `bay domain [add|remove|default] [name]` | List the machine's domains, or change them. | remove | the administrator password, in a system dialog or on the terminal, when a step needs root |
| `bay status [--json]` | Print the proxy, the DNS server and every registered project. | read | nothing |
| `bay doctor` | Report what machine setup is missing. Changes nothing. | read | nothing |
| `bay check [-scope all|run]` | Report each part of the machine setup and fail when any is missing. Changes nothing. | read | nothing |
| `bay peer [open|close|find|add|remove] [address|name|fingerprint]` | List the machines whose domains this one reaches, or change them. | remove | approval of the other machine's fingerprint, read from standard input; with no one to answer, nothing is approved |
| `bay sync` | Rewrite the proxy configuration from the containers that are running. | change | nothing |
| `bay cert client <name> [-out dir] [--json]` | Write a client certificate the machine's authority issues, for mutual TLS from outside the containers. | change | nothing |
| `bay install [-dry-run] [-only-from host,...|none]` | Apply the machine setup that is missing. Running it again changes nothing. | change | the administrator password, in a system dialog or on the terminal, when a step needs root |
| `bay uninstall` | Remove the machine setup install wrote. | remove | the administrator password, in a system dialog or on the terminal, when a step needs root |
| `bay brief [--json]` | Print the full usage contract on one screen. | read | nothing |
| `bay schema [--json]` | Print the Compose extension contract. | read | nothing |
| `bay skill [print|install]` | Print the agent skill, or write it where agents read skills. | change | nothing |
| `bay mcp [install]` | Serve the commands to an agent as an MCP server on standard input and output, or register it. | change | nothing |
| `bay help [command]` | Print help for one command, or the command list. | read | nothing |
| `bay version [--json]` | Print this executable's version, commit, Go version, platform and contract schema version. | read | nothing |

| Exit status | Meaning |
| --- | --- |
| `0` | The command did what it states. For check, every part of the setup is in place. |
| `1` | The command failed, or check found a part missing. Standard error says why and names the command that reports more. |
| `2` | The command line names no command or an unknown one. |

<!-- end generated -->

## 문서

| 문서 | 내용 |
| --- | --- |
| [설치](docs/operations/install.ko.md) | 1회 머신 설정과 제거 방법 |
| [사용](docs/operations/using.ko.md) | 프로젝트 추가, 도메인, 포트, 일상 명령 |
| [문제 해결](docs/operations/troubleshooting.ko.md) | 증상, 원인, 조치 |
| [구조](docs/spec/architecture.md) | 구성 요소, 요청 경로, 라우팅 상태 |
| [명령 계약](docs/spec/cli.md) | 명령, 효과, 규칙, 진단 |
| [Compose 계약](docs/spec/compose-schema.md) | 키, 라벨, 포트 결정 |
| [머신 상태](docs/spec/machine-state.md) | 저장소 밖에 생성되는 모든 경로 |
| [기능](docs/features.ko.md) | 구현 상태와 검증 |
| [변경 기록](CHANGELOG.ko.md) | 동작 변경과 검증 |
| [문서 계획](docs/documentation-plan.ko.md) | 어떤 정보를 어디에 적는가 |
| [피어](docs/spec/peers.ko.md) | 피어 링크, 릴레이, Linux 설정과 방화벽 |
| [에이전트 skill](skills/bay/SKILL.md) | 에이전트가 읽는 것. `bay skill print`로 생성 |
| [개발](AGENTS.md) | 빌드, 테스트, 필수 검사 |

명세 문서는 코드에서 생성되는 표를 담고 있어 영어만 유지한다.

## 요구 사항

Apple `container` 1.3 이상 또는 Docker가 설치된 macOS, Go 1.27 이상. 앱은 Xcode 명령줄
도구도 필요하다. Linux 피어는 Docker, systemd, 그리고 `-only-from`에는 nftables가 필요하다.

## 라이선스

MIT. [LICENSE](LICENSE) 참고.
