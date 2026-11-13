# 변경 기록

동작 변경과 각 변경에 대해 실행한 검증을 기록한다. 날짜는 변경한 날이다.

## 2026-11-14

### 작업: 한 번에 하나씩 돌리고 결과를 남기는 큐

`x-bay.job: true`인 서비스가 작업이다. `bay job add`로 실행을 큐에 넣으면 에이전트가 한 번에 하나씩, 넣은 순서대로 프로젝트의 나머지 서비스를 띄운 채 돌리고, 종료 코드, 걸린 시간, 메모리 최고치(Linux, Docker), 출력을 `~/.bay/jobs/`에 남긴다. `x-bay.schedule`은 `bay job schedule`로 프로젝트를 따르게 하면 cron 일정대로 큐에 넣고, `x-bay.timeout`은 한 번 실행의 상한이다(기본 1h, 최대 24h). `-key`는 키마다 한 번만 넣으므로, 연결이 끊겨 `add`를 다시 보낸 호출자도 실행은 하나다. 실행은 에이전트보다 오래 사는 별도 프로세스라, 재시작하거나 업데이트된 에이전트가 넘겨받는다(`bay install`이 쓰는 `bay-agent.service`의 `KillMode=process`). 승인한 peer는 `/__bay/jobs`로 작업을 제공하고, `bay job list|show|logs|wait -peer <name>`이 그것을 읽으며, `wait -peer`는 링크가 끊겨도 상한까지 다시 시도한다. macOS에서는 이 기계나 peer의 작업이 끝나면, 링크가 끊긴 동안 끝난 것까지 알림을 띄운다. `start`와 `restart`는 작업을 거부하고, 무엇도 작업에 의존할 수 없다. [작업](docs/spec/jobs.ko.md) 참고.

검증: `TestScheduleMatchesLikeCron`, `TestJobServicesAreLeftToTheQueue`, `TestQueueRunsOneJobAtATimeInOrder`, `TestSchedulesAreFollowedOncePerProject` 통과. Mac(Apple 엔진)에서 3으로 끝난 작업은 종료 코드 3의 `failed`, 8초 상한에 닿은 작업은 143의 `failed`, 취소한 작업은 `cancelled`였고, 큐의 작업은 순서대로 돌았다. 두 번째 `add -key k1`은 첫 작업의 ID를 출력하고 아무것도 넣지 않았다. 40초짜리 작업 15초 시점에 에이전트를 `launchctl kickstart -k`하자 `taking over its run`이 기록됐고, 작업은 40줄을 모두 남기고 `succeeded`로 끝났다. ODROID에서는 `bay install`이 유닛을 다시 쓴 뒤(`KillMode=process`) 작업 중 `systemctl restart bay-agent`에도 같았고, Mac의 `bay job wait -peer odroid`가 출력과 `succeeded in 51s, 8 MiB at most`를 보였다. `* * * * *` 일정은 매분 한 번씩, 각각 약 600 MiB로 돌았다. 실제 단절 상황에서의 재시도는 시험하지 않았다. 알림이 화면에 뜨는 것은 확인하지 않았다.

### `working_dir`를 적용한다

서비스의 `working_dir`는 읽고 무시됐다. 이제 엔진에 넘긴다(`--workdir`). 검증: `working_dir: /tmp`인 작업의 `pwd`가 Mac과 ODROID에서 `/tmp`를 출력했다.

### 명령이 이름을 따라잡는 에이전트를 실패 대신 기다린다

일정 작업이 에이전트가 이름을 쓰는 동안 `the machine setup is missing names in /etc/hosts (mac-hello.test, odroid-hello.test)`로 실패했다. `/etc/hosts`와 프록시 네트워크의 이름은 에이전트가 맞춘다. 그것만 뒤처진 것을 만난 명령은 실패하기 전에 에이전트를 최대 30초 기다리고, 한 번 그렇다고 말한다.

### 에너지 절약 이더넷은 그대로 둔다

아래 항목을 철회한다. Mac의 감시가 USB LAN 어댑터(USB 2.0의 Realtek RTL8153)가 07:55:32.96에 USB 버스에서 빠지고 ODROID 링크가 07:55:33에 내려간 것, 그리고 어댑터가 07:58:41부터 08:11:35까지 사라진 것을 기록했다. 단절은 ODROID의 PHY가 아니라 Mac의 어댑터에서 시작한다. 이제 `install`은 `bay-link.service`가 있으면 지우고 에너지 절약 이더넷을 다시 켠다. 검증: ODROID에서 `install`이 유닛을 지웠고 `ethtool --show-eee eth0`이 `enabled - inactive`를 읽었다. `TestRetiredLinkUnitIsRemovedWhereItIs` 통과.

### Linux 기계 업링크의 에너지 절약 이더넷을 끈다

ODROID의 링크가 00:55에 켜진 뒤로 쉬는 중에도 부하 중에도 열세 번 내려갔다 올라왔고, 한 번은 링크가 붙어 있다고 하면서 8분 동안 프레임을 주고받지 못했다. 인터페이스(stmmac + RTL8211F PHY)는 에너지 절약 이더넷이 켜져 있었고, 저전력 대기에서 깨어나기에 실패한 기록이 있었다(`phy_eee_wakeup_error_n: 1`). 이제 Linux의 `install`은 기본 경로를 가진 인터페이스의 에너지 절약 이더넷을 `bay-link.service`로 끄며, 이 유닛이 부팅 때마다 다시 적용한다. `check`는 `ethtool --show-eee`로 읽는다. 커널이 PHY를 직접 다루는 인터페이스(`/sys/class/net/<if>/phydev`)에만 적용하므로, CI 러너 같은 가상 인터페이스에는 해당하지 않는다. 적용할 때 링크가 한 번 다시 협상한다.

검증: ODROID에서 `install -dry-run`이 이 부분을 보였고 `install`이 `energy-efficient ethernet is off on eth0`를 출력했으며, `ethtool --show-eee eth0`이 `EEE status: disabled`를 읽었다. 링크는 한 번 다시 협상했고(07:26:09–07:26:12), 두 번째 `install`은 아무것도 바꾸지 않았으며 유닛을 재시작해도 링크는 붙어 있었고 `check`는 0으로 끝났다. 단절은 멈추지 않았다. 에너지 절약 이더넷을 끈 상태에서 깨어나기 실패가 더 없이 07:28:08에 링크가 다시 내려갔다 올라왔으므로 단절의 원인은 따로 있고 아직 모른다. 이 부분은 기록된 깨어나기 실패(링크가 붙은 채 프레임을 잃는다) 때문에 남긴다. `TestLinkPartOnlyForAPHYTheKernelDrives`가 통과한다.

### 부하가 걸린 Linux 기계가 네트워크에서 빠지지 않고 느려진다

ODROID(3.7 GiB)에서 프로젝트 컨테이너들이 메모리를 바닥까지 썼고(`kswapd0`가 `mm/page_alloc.c`에서 경고), 링크는 붙은 채로 기계가 8분 동안 네트워크에 답하지 않다가 링크가 내려갔다 올라오자 돌아왔다. 이제 Linux의 `install`은 두 부분을 더 적용하고 `check`와 `install -dry-run`이 이를 보고한다.

- `bay.slice`(`/etc/systemd/system/bay.slice`): bay가 Docker에 만드는 서비스 컨테이너는 모두 여기에 들어간다(`--cgroup-parent`). `MemoryHigh=70%`를 넘으면 커널이 컨테이너에서 회수하며 느리게 하고, `MemoryMax=80%`에서는 슬라이스 안에서만 죽인다. `MemorySwapMax=10%`가 스왑을 묶는다. 스왑은 흔히 zram이라 슬라이스에 계산되지 않는 메모리에 남으므로, 메모리와 스왑을 합쳐 90%를 넘지 않는다. 셸, Docker, 에이전트, 프록시, 네트워크 드라이버는 나머지를 쓴다. 슬라이스 등록 전에 만든 컨테이너는 다음 `up`에서 다시 만든다. Docker 엔진은 systemd cgroup 드라이버를 써야 하며, 아니면 `check`가 고칠 설정을 알려 준다.
- `/etc/sysctl.d/99-bay.conf`: 커널이 메모리의 1/64(32–256 MiB, ODROID에서는 22 MiB 대신 58 MiB)를 기다릴 수 없는 할당(네트워크 드라이버의 수신 버퍼 등)에 남겨 두고, 2%에서 회수를 시작한다(`vm.watermark_scale_factor = 200`).

`uninstall`은 둘을 지우고 남은 파일에서 커널 값을 다시 읽는다.

검증: ODROID에서 `install -dry-run`이 두 부분을 보였고 `install`이 적용했으며(`min_free_kbytes` 60292, 슬라이스 2.6/3.0/0.4 GiB로 활성) 두 번째 `install`은 아무것도 바꾸지 않았다. `/dev/urandom`을 계속 메모리에 읽어 들이는 서비스가 2640 MiB와 스왑 376 MiB에서 묶였고, 2분 동안 기계는 300 MiB를 남겼으며 ssh는 0.23초, ping은 1ms 안에 답했다. 스왑을 25%까지 허용했을 때는 압축되는 데이터에서도 82 MiB만 남았기에 스왑을 10%로 묶는다. `TestMemoryReserveFollowsTheMachine`, `TestMemoryReserveItemReadsTheLiveValues`, `TestContainersLeaveTheSliceAloneWhereTheHostHasNone`가 통과한다.

### 교체된 실행 파일을 돌리는 상주 프로세스는 갖춰진 것이 아니다

DNS 에이전트, relay 데몬, Linux 에이전트는 실행 파일이 바뀌면 끝나고 launchd나 systemd가 새것을 띄운다. 그 기능 이전 버전으로 시작한 프로세스는 옛 코드로 계속 돌았고, 작업 등록이 `install`이 쓸 것과 똑같아서 `check`는 통과했다. 이제 상주 프로세스는 실행 파일이 마지막으로 바뀐 뒤(아카이브나 복사로 되돌릴 수 없는 inode 변경 시각) 시작했을 때만 갖춰진 것이다. 아니면 `check`가 `runs an older …`로 알리고 `install`이 재시작한다(`restart the peer relay dev.bay.relay`).

검증: Mac에서 relay 데몬은 03:02에 시작했고 bayd는 06:05에 교체됐다. `check`가 빠졌다고 알렸고 `install -dry-run`이 재시작을 보였으며, `install`이 관리자 대화상자를 거쳐 재시작했고 `check`는 0으로 끝났으며 두 번째 `install`은 아무것도 바꾸지 않았다. `TestResidentStale`, `TestResidentStaleThisProcess`는 macOS와 ODROID(Linux, arm64)에서 통과했다.

### 동시에 도는 명령이 서로의 상태를 지킨다

`up` 여럿을 동시에 돌리면 등록에서 프로젝트가 사라졌다: 각자 `groups.json`을 읽고 고쳐 같은 `groups.json.tmp`로 다시 써서, 남의 임시 파일을 가져가거나(`rename … groups.json.tmp …: no such file or directory`) 남의 프로젝트가 빠진 등록을 써 넣었다(`project … is not registered`). 공유 파일(등록, 설정, 승인한 peer, 컨테이너가 신뢰하는 저장소)은 이제 `<state>/locks/`의 잠금 아래에서 읽고 고쳐 쓰며, 쓰는 쪽마다 다른 임시 파일을 쓴다. 프록시 게시(설정, 인증서, 컨테이너)는 두 번째 잠금을 잡아 두 명령이 함께 프록시를 만들지 않는다.

검증: Mac에서 `bay -p parN up -no-pull --json` 다섯 개를 동시에 돌려 모두 0으로 끝났고 각자 자기 프로젝트를 출력했으며 다섯 모두 등록됐다. `down` 다섯 개를 동시에 돌려 모두 0으로 끝났고 남은 컨테이너와 등록이 없었다. `TestConcurrentRegistrationsAreAllKept`는 프로젝트 24개를 동시에 등록한다.

### 곧바로 끝나는 작업, 터미널 없는 exec, 거부된 첫 실행

- `up`이 볼 때 이미 끝난 포트 없는 서비스는 완료로 보고 이름을 기다리지 않는다. 포트가 있는 서비스가 이름을 기다리는 중에 멈추면 60초 뒤(`hostname readiness failed: context deadline exceeded`)가 아니라 곧바로 실패한다.
- `exec`는 표준 입력이 터미널일 때만 터미널을 요청한다. `/dev/null`도 문자 장치라서 엔진이 거부했다(`Operation not supported by device`).
- 컨테이너를 만들기 전에 실패한 첫 `up`은 프로젝트 등록을 거둔다(`-no-pull`이 없는 이미지를 거부한 뒤에도 `status`에 남았다).

검증: Mac에서 `echo 1.0` 서비스와 대기 서비스가 있는 프로젝트가 4.6초에 exit 0으로 올라왔다. `bay exec db echo via-null </dev/null`이 출력하고 0으로 끝났다. `up -no-pull`이 없는 이미지를 거부한 뒤 `status --json`에 프로젝트가 없었다.

### 이미지 저장소를 덮지 않는 신뢰, 파일대로의 entrypoint와 restart

이 기계를 신뢰하는 서비스는 프로세스 앞에서 bayinit(`/etc/bay/init`에 마운트한 정적 프로그램)이 먼저 돈다: 인증기관(`/etc/bay/ca.crt`)을 이미지 자신의 저장소에 더하고, `update-ca-certificates`와 `update-ca-trust`가 읽는 원본 위치에도 두고, 이미지나 파일의 entrypoint와 command를 실행한다. `/etc/ssl/certs/ca-certificates.crt` 등 저장소 경로를 더는 덮어 마운트하지 않으므로 컨테이너 안에서 `ca-certificates` 설치가 된다(읽기 전용 묶음 때문에 실패했다). 변수는 `/etc/bay/ca-bundle.crt`를 가리킨다. 저장소를 쓸 수 없는 사용자의 컨테이너는 그대로 시작하고 변수로 신뢰한다. `make`가 bayinit을 linux/arm64와 amd64로 빌드해 내장한다.

Compose `entrypoint`가 이미지의 것을 대체한다(이미지 entrypoint의 인자로 넘어가고 있었다). 한 문자열로 쓴 command는 셸처럼 나눈다(따옴표 안도 공백에서 나뉘었다). `restart` 키가 Docker 재시작 정책이 된다. 없으면 포트가 있는 서비스는 `unless-stopped`, 없는 서비스는 정책 없음이라 끝난 작업이 다시 돌지 않는다(끝나고 3초 뒤 다시 시작됐다). 재시작 정책이 없는 Apple container에서는 `up`이 적용되지 않음을 경고한다.

검증: Mac에서 `debian:bookworm-slim` 서비스가 `apt-get install ca-certificates curl`을 마쳤고, 그 curl이 검증을 켠 채 `https://mac-hello.test`와 peer의 `https://odroid-hello.test`에 200을 받았다. 컨테이너 프로세스는 `/etc/bay/init sh -c …`였다. `TestBayinitRunsTheImagesProcess`, `TestRestartFollowsTheFile`, `TestACommandStringSplitsAsAShellWould`, 바뀐 `TestTrustArguments`가 통과하고 `make check`가 통과했다.

### 도구의 임무와 하지 않는 일을 명시하고, 하네스 보고를 고친다

`brief`, skill, README, `docs/spec/cli.md`가 bay가 하는 일과 하지 않는 일을 이유와 대안과 함께 적는다(`internal/contract/scope.go`): 다른 기계에서의 up/down, 호스트 포트와 Unix socket 공개, 한도 없는 대기, 호출자의 정책 결정, 프로젝트 명령의 설정 변경은 하지 않는다.

- `up`은 기계 설정을 적용하지 않는다. 빠진 부분과 `bay install`을 알리고 실패한다. 상태 디렉터리의 신뢰 저장소는 계속 갱신한다.
- 중단된 `up`은 만들거나 지우는 엔진 명령이 끝나기를 기다리고, 새 명령은 거부하고, 만든 것을 지운다. 정리 단계의 목록 조회가 중단에 같이 끝나던 문제를 고쳤다. ODROID에서 `creating its container` 중 SIGTERM 3회 모두 남은 컨테이너가 없었다(전에는 1~2개가 남아 두 번째 `down`에서야 지워짐).
- 포트가 없는 서비스는 기다리지 않는다. 일회성 작업이 있는 `up`이 20초 뒤 경고 대신 3.6초에 끝났다.
- 멈춘 컨테이너의 `logs -f`는 출력하고 끝나며, 따라가던 컨테이너가 멈추면 끝난다.
- 프록시가 이미 멈춰 있던 `down`은 프록시를 다시 띄우지 못해도 성공하고 프록시 상태를 알린다. Compose 검증에 실패한 파일의 `down`은 프로젝트 이름만 읽어 exit 0으로 끝난다.
- 컨테이너가 멈추면 healthcheck가 1초 안에 실패한다. `retries` 범위 1~100과 한도를 두는 이유를 문서에 적었다.
- Docker 응답 확인을 `docker version`으로 한다. 부하가 큰 ODROID에서 `docker info`가 20초 걸려 응답 없음으로 보고됐다.
- 새 기능: `bay exec`(명령 자신의 exit 상태), Compose `tmpfs` 키(Apple container에서는 옵션을 이름으로 거부), `bay plan [--json]`(파일만으로 이름, 포트, 인증서 경로).
- 대시보드가 승인한 peer의 CPU, 메모리, swap 사용을 5초마다 읽고 다음 읽기까지 남은 시간을 센다. peer 링크의 `/__bay/stats`는 승인한 인증기관이 발급한 클라이언트 인증서에만 답한다. `bayd`는 peer 문서 옆에 부하를 쓰고, 실행 파일이 바뀌면 끝나서 launchd와 systemd가 새 버전을 띄운다. `make install`은 떠 있는 대시보드를 새 빌드로 다시 연다.

검증: `make check` 통과. Mac: `exec`가 `exit 7`에 7을 돌려주고 파이프 입력과 `-e`/`-w`를 적용했다. `tmpfs`가 마운트됐다. `-p other`에서 `plan --json`이 `other-db.bay.test`를 냈다. 클라이언트 인증서를 쓴 `curl`이 ODROID와 Mac의 부하를 읽었고, 인증서 없이는 403이었다. ODROID: `bayd` 교체 후 에이전트가 스스로 재시작했다(systemd 재시작 횟수 1).

### 에이전트가 최신 skill과 MCP 서버를 찾는다

`bay mcp install`과, `claude` 명령이 있으면 `install`이 실행 파일의 절대 경로로 Claude Code 사용자 설정에 MCP 서버를 등록하며, 등록이 다를 때만 바꾼다. `check`는 없거나 낡은 등록을 알린다. `make install`과 시작하는 MCP 서버가 바이너리와 다른 skill을 다시 쓴다.

검증: `make install`이 skill을 쓰고 `/opt/homebrew/bin/bay mcp`를 등록했다. 두 번째 `bay install`은 아무것도 바꾸지 않았고 `check`가 둘 다 `ok`로 보고했다. `TestTheMCPRegistrationFollowsTheExecutable` 통과.

### 에이전트가 계약에서 도구를 배운다: skill, MCP 서버, README

모든 명령이 접근 수준(`read`, `change`, `remove`)과 사람에게 묻는 것을 선언하고, 계약에 `schemaVersion`과 종료 상태(0 완료, 1 실패 또는 설정 누락, 2 알 수 없는 명령. 전에는 알 수 없는 명령이 1로 끝났다)가 들어간다. 이 하나의 선언에서 `bay skill print`가 SKILL.md를 만들고, `bay skill install`과 `install`이 `~/.claude/skills/bay/`과 `~/.agents/skills/bay/` 중 디렉터리가 있는 곳에 내용이 다를 때만 쓰며, `check`가 없거나 낡은 skill을 알린다. `bay mcp`는 터미널 전용이 아닌 모든 명령을 접근 수준에 따른 읽기 전용·파괴적 표시를 단 MCP 도구로 제공하고(이 실행 파일을 주어진 디렉터리에서 빈 표준 입력으로 실행), `brief`, `schema`, `status`를 리소스로 제공한다. README의 에이전트 섹션과 `skills/bay/SKILL.md`는 `make docs-generate`가 생성하고 `make check`가 검사한다. 규칙과 머신 상태 목록에 릴레이 데몬, Linux 설정, 컨테이너 신뢰 저장소를 넣었다.

검증: `make check` 통과. `bay skill install`을 두 번 실행하자 두 번 다 `current`를 출력했고 파일 수정 시각이 바뀌지 않았다. 표준 입력으로 `bay mcp`가 도구 16개를 나열했고 `check` 도구가 오류 없이 check 출력을 돌려줬다. `TestMCPToolsFollowTheContract`, `TestMCPToolRunsTheCommand`, `TestSkillsAreWrittenWhereAgentsAre`, `TestCommittedSkillIsCurrent` 통과. 테스트는 더 이상 홈 디렉터리에 쓰지 않는다.

### 모든 서비스 컨테이너가 머신이 신뢰하는 것을 신뢰한다

컨테이너 안 프로그램은 이미지의 인증서 저장소로 검증했으므로, 이 머신과 피어가 제공하는 이름(이 머신의 인증기관이 발급한 인증서)은 그 안에서 검증에 실패했다. 이제 `install`과 `up`이 `<state>/trust/`를 쓴다. 시스템 공인 루트와 인증기관(`ca-bundle.crt`), 인증기관 단독, Java용 PKCS #12 저장소이고, 내용이 바뀔 때만 파일을 제자리에서 다시 쓴다. 각 서비스 컨테이너는 묶음을 `/etc/ssl/certs/ca-certificates.crt`, `/etc/pki/tls/certs/ca-bundle.crt`, `/etc/ssl/cert.pem`에 읽기 전용으로, Java 저장소를 `/etc/bay/truststore.p12`에 마운트하고, `SSL_CERT_FILE`, `CURL_CA_BUNDLE`, `REQUESTS_CA_BUNDLE`, `NODE_EXTRA_CA_CERTS`, `JAVA_TOOL_OPTIONS`를 갖고 만들어진다. 서비스가 직접 정한 경로나 변수는 그대로 두고, `x-bay.trust: false`가 끈다. 이 인자는 설정 식별값에 들어가므로 인자 없이 만들어진 컨테이너는 다시 만들어진다. Apple `container`가 파일 마운트의 원본 폴더를 함께 폴더째 마운트하면 폴더 마운트를 버렸기 때문에 모든 마운트는 파일 하나씩이다. `check`가 저장소를 보고한다.

검증: Mac(Apple `container`)과 ODROID(Docker)의 `alpine:3.20` 서비스에서 검증을 켠 `wget`이 `https://mac-hello.test`, `https://odroid-hello.test`, `https://example.com`을 받았다. ODROID의 `eclipse-temurin:21-jdk-alpine` 서비스에서 Java `HttpsURLConnection`이 셋 모두 200으로 답했다. 두 번째 `install`은 아무것도 바꾸지 않았다. 식별값에 인자를 넣기 전에는 인자가 바뀌어도 옛 컨테이너가 남아 모든 요청이 검증에 실패했고, `TestTrustArgumentsArePartOfTheIdentity`가 이를 검사한다. 전체 실행에서 실패한 프록시 테스트 4개(`TestARouteIsPublishedAfterTheProxyConnectsToIt` 외 3개)는 변경 없는 `HEAD`에서도 실패했다. `syspolicyd`가 막혀 있는 동안 새로 쓴 두 줄 스크립트가 첫 실행에 80초와 44초, 두 번째 실행에 0초가 걸려 엔진 읽기 제한 15초를 넘었다. 이제 fixture가 시간을 재는 호출 전에 엔진 스크립트를 한 번 실행하고, 4개 모두 통과한다.

### `install`은 빠진 것만 적용하고, `check`와 `install -dry-run`이 그것을 보고한다

`check`, `doctor`, `install -dry-run`은 설치 항목 목록 하나를 읽고, `install`은 빠진 항목만 적용한다. 그래서 다시 실행하면 아무것도 바꾸지 않는다. `check`는 항목마다 한 줄을 출력하고 빠진 것이 있으면 1로 끝난다. `install -dry-run`은 `install`이 바꿀 것을 출력하고 아무것도 바꾸지 않는다. macOS가 아닌 머신의 항목은 인증기관, Docker 엔진(보고만 하고 설치하지 않는다), 그리고 `install -only-from <호스트>`를 주었을 때 `bay-firewall.service`가 올리는 nftables 방화벽이다. 방화벽은 기본 경로의 인터페이스에서 그 호스트의 새 연결만 받는다. 호스트는 `machine.json`에 기록된다.

### macOS가 아닌 머신이 피어의 이름에 닿는다

Linux에서 `install`은 `bay-agent.service`(`-hosts`를 준 bayd)를 등록하고, 시스템이 가진 인증서 저장소에 인증기관을 넣는다. 에이전트는 `/etc/hosts`의 표시된 블록에 모든 경로와 승인한 피어의 도메인을 127.0.0.1로 적어 둔다. 시스템에 저장소, systemd, nft가 없으면 `check`와 `doctor`가 무엇을 설치해야 하는지 보고하고, install은 그 항목에 대해 아무것도 바꾸지 않는다. macOS에서 터미널이 없으면 관리자 단계는 실패하지 않고 시스템 대화상자로 묻는다.

Docker 프록시 네트워크의 컨테이너는 같은 이름을 Docker DNS로 해석한다. 프록시는 제공하는 모든 이름을 네트워크 별칭으로 갖고, 목록이 다를 때만 네트워크에 다시 연결해서 바꾼다. `check`가 이 이름을 보고하고, `install`과 모든 프록시 재작성이 적용한다.

검증: ODROID에서 `check`가 네트워크의 이름이 빠졌다고 보고했고, `install -dry-run`이 프록시 재작성을 나열했고, `install`이 적용했고, 두 번째 `install`은 `ok` 외의 줄을 출력하지 않았고, 네트워크의 `alpine:3.20` 컨테이너에서 `nslookup mac-hello.test`가 프록시 주소 172.18.0.3을 답했고 `wget https://mac-hello.test`가 Mac 서비스 페이지를 돌려주었다(`--no-check-certificate` 사용). ODROID에서 `install -dry-run`이 인증기관 추가를 나열했고, `install`이 추가했고, 두 번째 `install`은 `ok` 6줄을 출력하고 아무것도 바꾸지 않았고, `curl https://mac-hello.test`(Mac의 서비스)와 `curl https://odroid-hello.test`가 `--cacert` 없이 답했다. Mac에서 터미널 없이 `bay install`이 대화상자로 relay 데몬을 등록했고, `curl https://odroid-hello.test`가 relay를 거쳐 200으로 답했다. `TestTheTrustStoreIsReadFromTheSystem`, `TestWithoutSystemd`, `TestTheHostsBlock`, `TestAdminScriptQuotesEveryArgument`가 통과한다.

### Apple 프록시는 relay 데몬을 거쳐 피어에 연결한다

Mac의 Apple 프록시는 기본 경로가 아닌 네트워크의 피어에 연결하지 못했다. 피어가 컨테이너 주소로 응답했고 그 응답이 돌아오지 않았다(`504`). 이제 프록시는 `192.168.65.1:8444`에 연결하고, launchd 데몬 `dev.bay.relay`가 TLS 이름을 읽고 Mac에서 승인된 피어로 연결한다. macOS가 사용자 에이전트의 연결을 묻지 않고 `no route to host`로 거부했기 때문에 relay는 데몬이다. `install`이 등록하고 `uninstall`이 제거한다.

### Docker가 프로젝트의 서비스를 실행한다

엔진이 Docker인 머신에서는 Docker의 명령과 형식으로 서비스를 나열하고 만들고 검사한다. 서비스와 프록시는 `--restart unless-stopped`를 갖는다. 다른 머신의 Docker endpoint는 이 머신의 엔진으로 세지 않는다. macOS가 아닌 머신에는 resolver 항목, DNS 에이전트, 신뢰 설정이 없다.

검증: `go test ./...`와 `make check`가 통과한다. `TestTheRelayPassesTheSessionToThePeerServingTheName`, `TestTheRelayListensOnlyOnThisMachineAtItsPort`, `TestTheRelayIsPartOfTheMacSetup`, `TestFirewallRules`, `TestEnsureInstalledOnAMachineThatIsNotMacOS`, Docker 해석 테스트를 포함한다. ODROID-N2+(DietPi, Docker 29.9.3)에서 `bay install -only-from 192.168.2.1`이 방화벽을 쓰고 올렸고, 두 번째 `install`은 아무것도 바꾸지 않았고, `check`는 0으로 끝났고, 재부팅 뒤에도 서비스 컨테이너가 실행 중인 채로 `check`가 0으로 끝났다. Mac에서 launchd 에이전트가 ODROID로 연결하면 `no route to host`로 실패했고 같은 연결을 터미널에서 하면 성공했다. `check`는 relay 데몬을 빠졌다고 보고하고 `install -dry-run`은 그 등록을 나열한다.

## 2026-11-10

### `up`의 volume 명령마다 자기 bound가 있다

`up`은 선택된 service의 모든 `container volume inspect`와 `container volume create`를 1분 deadline을 가진 context 하나 아래에서 실행했으므로, managed volume 약 40개의 stack이 deadline을 넘었고, 정상인 create가 끝났으며, error는 끝난 process의 빈 standard error를 가진 `container volume:`이었다. 이제 각 volume inspect는 15초의 자기 read bound 아래에서, 각 volume create는 10분의 자기 action bound 아래에서 실행된다. context가 끝낸 service 명령은 명령, context error와 경과 시간으로 실패한다.

검증: `TestEachVolumeCommandHasItsOwnBound`가 변경 전에 `volume inspect runs under 1m0s, want its own bound 15s`와 `a healthy volume command was ended: container volume: context deadline exceeded after 1m0s`로, `TestServiceCommandNamesTheEndOfItsContext`가 `error = "container volume: "`로 실패했고, 변경 후 둘 다 통과한다. `make check`가 통과한다.

## 2026-11-09

### `bayd`는 새 목록을 만드는 동안 컨테이너의 마지막 목록으로 답한다

`bayd`는 목록이 TTL보다 오래되면 lock을 쥔 채 `container ls`를 실행했으므로, 모든 질의가 최대 15초가 걸린 그 목록을 기다렸고, 답을 받지 못한 resolver는 그 이름을 실패시켰다. 이제 TTL보다 오래된 목록은 새 목록 하나를 background에서 시작하고, 새 목록이 마지막 목록을 바꿀 때까지 모든 질의에 마지막 목록으로 답하며, 실패한 새 목록은 마지막 목록을 남기고 걸린 시간과 함께 log에 기록된다. 첫 목록이 생기기 전의 질의만 그것을 기다린다(architecture 규칙 11).

검증: `TestIndexAnswersFromLastListWhileNewListRuns`가 변경 전에 `index waited for the new list for more than 100 ms`로 실패했고 변경 후 통과한다. `TestRefreshKeepsLastListWhenNewListFails`와 `TestRefreshReplacesLastList`가 통과한다. `go test -race ./cmd/bayd`와 `make check`가 통과한다.

## 2026-10-13

### `up`은 선언되지 않은 서비스의 컨테이너를 제거한다

Compose 파일에서 제거된 서비스는 `up`이 선언된 서비스만 시작하므로 컨테이너와 라우트를 그대로 유지했다. 이제 `up`은 프로젝트의 서비스 컨테이너 중 service label을 파일이 선언하지 않는 것을 찾아, 실행 중이면 기록된 유예 기간으로 중지하고 제거하며 각 제거를 보고한다. 라우트는 시작 후 프록시를 동기화할 때 철회한다. 다른 프로젝트, 프록시와 선언된 서비스는 바꾸지 않는다.

검증: 선언되지 않은 컨테이너 테스트 3개는 제거 대신 스텁을 둔 변경 전에 실패했고, 변경 후 통과한다. `make check`가 통과한다. 변경은 limepie-hyper 컨테이너에 대해 실행하지 않았다.

### `stop_signal`으로 서비스를 중지할 신호를 정한다

`stop_signal`을 가진 서비스는 그 신호로 중지한다. 허용하는 이름은 SIGHUP, SIGINT, SIGQUIT, SIGTERM, SIGUSR1, SIGUSR2, SIGKILL이다. 그 밖의 값은 파일을 읽을 때 서비스와 값을 함께 보고하며 거부한다. 신호는 컨테이너에 `bay.stop_signal`로 기록하고, 교체, stop, restart, 실패한 초기화의 중지와 down에서 `container stop --signal <이름>`으로 전달한다. 키가 없는 서비스는 `--signal`을 받지 않는다. `bay schema`는 키를 나열한다.

검증: 기능 표의 신호 테스트는 변경 전에 실패했고, 변경 후 통과한다. `make check`가 통과한다. 변경은 실제 컨테이너에 대해 실행하지 않았다.

## 2026-10-11

### 서비스는 `stop_grace_period`로 중지한다

`stop_grace_period`(예: `30s`, `1m30s`)를 가진 서비스는 그 유예 기간으로 중지한다. 교체, `stop`, `restart`와 실패한 초기화의 중지는 기간을 `container stop --time <초>`로 전달하고, `down`은 기록된 기간으로 실행 중인 컨테이너를 제거 전에 중지한다. 키가 없는 서비스는 런타임 기본값 5초를 그대로 사용한다. 초 단위 정수가 아니거나, 음수이거나, 기간이 아닌 값은 파일을 읽을 때 서비스와 값을 함께 보고하며 거부한다.

검증: 기능 표의 중지 경로와 잘못된 값 테스트는 변경 전에 실패했고, 변경 후 통과한다. `make check`가 통과한다. 변경은 실제 컨테이너에 대해 실행하지 않았다.

## 2026-10-06

### 호스트 이름 준비 검사는 cache된 negative answer를 기다리지 않는다

컨테이너가 존재하기 전에 이름을 조회한 새 서비스에서 `bay up`이 60초 후 `hostname
readiness failed: context deadline exceeded; received []`로 실패했다. runtime DNS는 아직 없는
이름에 SOA 없는 NXDOMAIN을 반환하고, system resolver는 그 응답을 77초 동안 유지한다. 준비 검사는
DNSServiceGetAddrInfo로 구독했는데, 이 구독은 그 항목에서 응답받고 negative answer를 받지 않으므로
기한 전에 주소를 받지 못했다. 컨테이너는 같은 cache에서 응답하는 network gateway를 통해 조회하므로
의존 서비스도 그 이름을 조회하지 못했다.

이제 준비 검사는 system resolver에 대한 connection 하나에서 A와 AAAA 각각에 대한 long-lived
DNSServiceQueryRecord query를 가진다. long-lived query는 질문을 runtime DNS server로 보내고,
server의 응답이 그 이름의 cache 항목을 대체한다. 준비 기한은 변경하지 않았다.

검증: `BAY_E2E=1 go test ./internal/stack -run
TestNativeNameSubscriptionReplacesCachedNegativeAnswer`가 변경 전에는 20초 한도 후 `received []`로
실패했고 변경 후에는 통과한다. 주소가 10ms 후 도착하고 그 뒤 system resolver가 그 주소를 반환한다.
`make check`. 다른 end-to-end 테스트는 실행하지 않았다.

배포: 변경 없는 소스 `e57c129`에서 `make install PREFIX=/opt/homebrew`.
`/opt/homebrew/bin/bay`이 리비전 `e57c129`와 `vcs.modified=false`를 보고한다.

## 2026-10-03

### 서비스 상태는 엔진의 상태다

선언한 포트가 연결을 거부하는 실행 중 컨테이너를 `starting · 아직 연결을 받지 않음`으로 보고했다.
`soksakim-hyper`의 `php`가 감독 프로세스가 php-fpm을 정지시킨 동안 그렇게 표시됐다. 시작 중인
것은 없었고, 그 단어는 측정하지 않은 상태를 나타냈다. 이제 상태는 엔진의 상태(`running`,
`stopped`, `absent`)이고, 포트 측정은 상태와 따로 포트 번호와 함께 보고한다. `bay status`는
`port 9000 does not accept connections`를 덧붙이고, 창은 같은 문장을 쓰며,
`bay status --json`에는 `accepting`이 있다. 포트가 없거나 실행 중이 아닌 서비스에는 없다.

이제 실패는 서비스 로그가 어떻게 되었는지 보고한다. 로그의 마지막 줄들, `its log is empty`, 또는
엔진의 이유와 함께 `its log could not be read`다. 뒤의 둘은 아무 말 없이 빠졌다.

`make install`이 설치된 바이너리를 제자리에서 덮어썼고, 그 뒤 커널이 `bay`을 SIGKILL로
종료했다(종료 코드 137). 이제 복사하기 전에 지운다.

검증: `make check`, `make install PREFIX=/opt/homebrew` 두 번, 이 기계에서 `bay status --json`과
포트 9000에 대한 `nc -z`, 그리고 `make window`로 캡처한 창. 이 기계에 닫힌 포트가 없어 그 경우는
관찰하지 못했다. 종단 테스트는 바꿨고 실행하지 않았다.

### 포트 없는 서비스에는 포트를 쓰지 않는다

직전 변경이 포트를 선언하지 않은 서비스에 값 0을 주고 그대로 내보냈다. 서비스 화면에 "Compose
파일에서 읽음" 옆 `0`, `:0`으로 끝나는 주소, `bay status --json`의 `"port": 0`이다.
0은 포트가 아니다. 이제 그 서비스 화면에는 포트 행이 없고, 주소는 이름뿐이며, JSON에는 `port`
필드가 없다. `bay brief`와 `schema`는 여전히 포트 순서를 80으로 끝냈다. 이제 없음으로
끝내고, 그 서비스는 확인하지 않으며 도메인이 있는 서비스는 거부한다고 밝힌다.

검증: `make check`, 그리고 이 기계에서 JSON과 `bay schema`.

### 포트는 가정하지 않는다

포트를 선언하지 않은 서비스에 포트 80을 주었다. `soksakim-hyper-test`의 `node`는
`sleep infinity`를 실행하며 포트를 선언하지 않았는데, 80으로 확인되어 연결을 한 번도 받지
않았고, 자기 포트로 연결을 받지 않는다며 주황으로 보였다. 화면이 bay이 지어낸
값으로 고장을 보고했다.

이제 서비스의 포트는 파일이 선언한 포트이고, 선언하지 않은 서비스는 포트가 없다. 연결을 확인하지
않으며 컨테이너가 실행 중이면 실행 중이다. 도메인이 있는 서비스는 여전히 프록시가 쓸 포트가
필요하므로, 선언하지 않았으면 파일을 읽을 때 서비스와 도메인을 밝히며 거부한다. 실행 중인 서비스가
연결을 받는지는 지금 파일이 선언한 포트로 확인한다. 예전에 만든 컨테이너의 레이블에는 가정한
80이 들어 있을 수 있다. 이 저장소의 예시 서비스는 80으로 듣는데 아무것도 선언하지 않았으므로
이제 `expose: ["80"]`을 선언하고, 포트 없이 도메인을 제공하던 테스트 픽스처도 그 이미지가 듣는
포트를 선언한다.

검증: `make check`. 이 기계에서 `node`가 `running`으로 읽히고 프로젝트는 초록, `4/4 실행 중`으로
보인다.

### 점, 개수, 버튼이 한 규칙을 따른다

주황이 두 가지를 뜻했다. 서비스에서는 실행 중이지만 선언대로 동작하지 않음을, 프로젝트에서는
모든 서비스가 준비되지는 않았음을 뜻했으므로, 서비스 셋이 멈춰 있기만 한 프로젝트가 고장처럼
주황으로 읽혔다. 개수는 연결을 받는 서비스를 세고 버튼은 실행 중인 컨테이너를 셌으므로, 모든
컨테이너가 실행 중인 프로젝트가 `3/4 실행 중`으로 읽히면서 정지를 보여주었고, 일부만 실행 중인
다른 프로젝트는 나머지 시작을 보여주었다.

이제 주황은 어디에 그려지든 실행 중이지만 선언대로 동작하지 않음을 뜻한다. 프로젝트는 실행 중인
서비스 중 하나가 주황이면 주황, 실행 중인 서비스가 모두 동작하면 초록, 실행 중인 것이 없으면
회색이다. 멈춘 서비스는 고장이 아니다. 개수는 컨테이너가 실행 중인 서비스이고 버튼은 그 개수를
따른다. 실행 중이지만 자기 포트로 연결을 받지 않는 서비스는 개수에 들어가고 그 이유로 이름이
나온다. 예전에는 아직 수신하지 않는다며 개수에서 빠졌다.

검증: `make check`. 이 기계가 보고한 그대로의 두 프로젝트를 검사하며, 창에서도 위와 같이
읽힌다.

### 목록 행 끝의 버튼은 폭 하나를 함께 쓴다

버튼마다 라벨만큼의 폭이었으므로, 나머지 시작을 보여주는 행은 정지나 시작을 보여주는 행보다
상태 글자가 왼쪽으로 밀렸고 목록의 글자가 줄을 맞추지 않았다. 이제 목록 행 끝의 같은 자리
버튼은 가장 넓은 버튼의 폭을 함께 쓴다.

검증: `make check`, 그리고 창이 활성일 때 찍은 대시보드 프로젝트 목록.

### 긴 목록은 화면에 있는 행만 배치한다

바뀌지 않은 화면을 다시 만들지 않게 된 뒤에도 인증서 화면을 여는 데 2.5초가 걸렸고, 그
전부가 화면에 없는 행의 배치였다. 이제 행이 마흔 개를 넘는 목록은 화면에 있는 행만 만들고
스크롤할 때 행을 다시 쓰는 표다. 그 행은 짧은 목록과 같은 방법으로 만들므로 같게 보이고 같게
반응한다. 더 짧은 목록은 행을 쌓은 그대로이며, 그 길이에서는 표보다 비용이 적다.

검증: `make check`. 인증서 화면을 여는 데 2,500ms에서 121ms가 되었고(임시 로그로 쟀고 그
로그는 지웠다), 마지막 인증서까지 스크롤하자 모든 행이 구분선과 버튼을 갖추어 그려졌다.

### 바뀌지 않은 화면은 다시 만들지 않는다

인증서 화면이 스크롤되지 않았다. 창은 3초마다 머신을 다시 읽고 그때마다 화면 전체를 다시
만들었다. 인증서가 283개일 때 레이아웃을 강제해 재니 다시 만드는 데 2.8~3.6초가 걸렸으므로
메인 스레드가 쉬지 않고 다시 만들고 있었다. 창을 샘플링하니 메인 스레드의 95%가 Auto Layout
솔버에 있었다. 인증서 목록의 넓은 이름 열을 꺼도 그대로였으므로, 비용은 제약 하나가 아니라
행의 수다.

창은 직전에 만든 패널과 같은 패널을 다시 만들지 않는다. 인증서 화면에서 15초 동안 창이 한 번
2.5초에 만들고 그 뒤 읽기마다 건너뛰었으며, 화면이 스크롤된다. 화면을 열 때는 여전히 그 한
번이 걸린다. 긴 목록에서 보이는 행만 배치하도록 그리는 것이 남은 변경이다.

검증: `make check`, 그리고 위의 시간. 시간은 임시 로그로 쟀고 그 로그는 지웠다.

### 도메인을 여러 개 제공하는 서비스는 한 행이다

프로젝트 목록이 도메인마다 행을 그리며 서비스의 이름, 점, 주소를 되풀이했으므로 컨테이너
하나가 서비스 셋으로 읽혔고, 사용량이 첫 행에만 있어 행들이 서로 달라 보였다. 이제
서비스는 한 행이고 그 도메인들을 위아래로 나열한다. 첫 줄은 도메인 하나인 행이 그리는
줄과 같다. 나머지 도메인은 첫 도메인의 왼쪽 끝에 맞춰 19포인트 간격으로 놓이고, 마지막
아래 여백은 첫 위 여백과 같다.

처음에는 행 전체를 위쪽에 맞췄다. 창에서 재 보니 첫 줄의 점, 이름, 버튼이 최대
7.5포인트 떨어진 세 중심선에 놓였고 마지막 도메인이 구분선에 닿았다.

검증: `make check`, 그리고 `node-web` 행에서 `tools/window.swift`. 첫 줄이 도메인 하나인
행처럼 구분선에서 35.5~37.5픽셀 아래, 나머지 도메인은 38픽셀 간격, 위아래 여백이 같다.

### 창이 실행 중인 각 컨테이너의 사용량을 보여준다

서비스 화면은 라우트, 컨테이너, 출력만 보여주고 사용량은 보여주지 않았다. 이제 실행 중인
서비스 화면은 `container stats`에서 읽은, 받은 코어 대비 CPU, 한도 대비 메모리, 디스크
입출력, 네트워크, 프로세스를 보여주고, 프로젝트 목록은 실행 중인 각 서비스 행에 CPU와
메모리를 보여준다. CPU는 두 번 읽은 사이 CPU 시간의 변화이므로 두 번째 읽기부터 나온다.

`container stats` 한 번이 컨테이너 1개, 4개, 10개 어느 쪽을 지목해도 약 2초 걸렸으므로,
프로젝트의 실행 중인 컨테이너는 머신 갱신과 별도로, 화면이 열린 프로젝트에 대해 한 번에
함께 읽는다. 처음에 20개가 실행 중일 때 모든 컨테이너를 읽는 데 43초가 걸렸는데, 이는
재현되지 않았고 설계의 근거가 아니다.

컨테이너별 디스크 용량은 보여주지 않는다. 어떤 명령도 알려주지 않으며, 유일한 값은 엔진
자체 저장 디렉터리 안 파일의 크기인데 엔진은 그 구조를 공개하지 않는다. 처음에는 값이
없다고 말하는 행을 보여주었다가 지웠다. 읽는 사람에게 아무것도 알려주지 않았다. Docker
통계는 읽지 않으며 Docker 서비스 화면은 그렇다고 말한다. 크기는 이진 단위로 쓴다.
컨테이너의 메모리 한도가 그 단위로 정해지기 때문이다.

검증: `make check`. 실측 출력에서 읽기, CPU 비율, 되돌아간 카운터, 행을 검사한다. 이
기계에서 `soksakim-hyper` 목록과 그 서비스 화면이 값을 보여주었다.

### 정지는 멈추고 제거는 지우며, 둘 다 한 일을 말한다

프로젝트의 정지가 `down`을 보냈다. `crudui`에서 누르자 컨테이너와 등록이 지워졌고,
프로젝트가 사이드바에서 빠져 되돌릴 시작 버튼도 없었다. 도는 동안 화면에는 꺼진 버튼
말고 아무것도 없었고, 끝나자 `proxy reloaded with 11 route(s)`가 나왔다.

이제 정지는 프로젝트의 서비스를 멈추고 컨테이너와 등록은 그대로 둔다. 제거는 따로 있는
컨트롤이며, 먼저 묻고 컨테이너와 함께 무엇이 지워지고 무엇이 남는지 밝힌다. 동작이 도는
동안 그 화면은 닿는 단계를 차례로 보여주고, 끝나면 무엇에 무엇을 했는지 말한다.
`aa-probe을(를) 정지했습니다`. 동작이 그 화면의 대상을 지우면 창은 대시보드를
보여주고 결과도 거기 보인다.

단계 줄은 명령줄의 문장 그대로라 한국어 창에서 영어로 나온다. 여기서는 고치지 않았다.

검증: `make check`. 컨트롤, 결과, 선택을 검사한다. 이 기계에서 시험 프로젝트에 정지와
제거를 눌렀고 위와 같이 동작했다.

### 사이드바의 프로젝트가 따로 열리고 닫히며 사이드바가 스크롤된다

프로젝트의 서비스는 그 프로젝트가 선택된 동안에만 나열됐고, 프로젝트를 다시 눌러도
아무 일이 없었다. 사이드바는 스크롤되지 않아서, 서비스가 열두 개인 프로젝트가 그 아래
행들을 창 밖으로 밀어내 닿을 방법이 없었다.

이제 프로젝트마다 따로 열리고 닫힌다. 선택되지 않은 프로젝트를 누르면 선택하면서 연다.
선택된 프로젝트를 누르면 열거나 닫는다. 서비스를 선택하면 그 프로젝트가 열린다. 다른
프로젝트를 선택해도 열린 프로젝트는 열린 채로 남는다. 사이드바는 스크롤 뷰 안에 있다.

검증: `make check`. 각 경우를 검사한다. 이 기계에서 `platform-project`를 누르니
서비스 열두 개가 나열됐고, 다시 누르니 닫혔으며, 사이드바를 스크롤해 창 아래에 있던
행들에 닿았다.

### 동작의 결과는 그 동작을 시작한 화면에 남는다

창이 모든 화면에 메시지 하나만 들고 있었다. `crudui`에서 실패한 정지가 그 뒤에 연 모든
화면에 보였고, `platform-sdk-local` registry 화면에서는 그 서비스의 실패로 읽혔다.
이제 결과는 그 동작을 시작한 화면의 것이며 다른 화면에는 보이지 않는다. 그 화면으로
돌아오면 다음 동작 전까지 다시 보인다.

검증: `make check`. 다른 화면과 동작 자신의 화면을 검사한다. 변경 후 화면에서는
확인하지 않았다.

### 프로젝트를 멈추는 데 Compose 파일이 필요 없다

프로젝트 컨테이너가 실행 중인데 창의 정지가 `down 실패: stat …/compose.yaml: no
such file or directory`를 보고했다. 멈추기와 지우기가 어떤 서비스를 멈출지 알려고
Compose 파일을 읽었으므로, 파일을 옮긴 프로젝트는 멈출 수 없었고 시작한 뒤 파일에서
뺀 서비스는 `down` 뒤에도 계속 실행됐다.

이제 멈출 대상은 컨테이너에서 읽는다. 프로젝트의 `bay.group` 레이블을 단
모든 서비스 컨테이너이며, 이름이 아니라 레이블로 고른다. 각 컨테이너의 시작 시각에서
읽은 시작 순서의 역순으로 멈추며, 이는 시작할 때 따른 의존 순서의 역순이다.
명령줄은 파일을 읽을 수 없을 때 등록부에서 프로젝트 이름을 찾는다. 이름으로 멈추던
경로는 지운다.

검증: `make check`. 레이블로 지우기, 파일에 더는 없는 컨테이너, 멈추는 순서, 지정한
서비스, 다른 프로젝트 레이블의 컨테이너를 그대로 두기, 등록부에서 이름 찾기를
검사한다. 이 기계에서 프로젝트를 시작하고 파일을 지운 뒤 `down`이 컨테이너와 등록
항목을 지웠다.

### 터미널 밖에서 시작한 창도 엔진을 찾는다

터미널에서는 `container`가 응답하는데 창은 `머신 상태를 읽지 못함 - no container
engine found: install Apple container or docker`를 보고했다. macOS가 터미널 밖에서
시작한 프로그램, 즉 Finder나 Dock에서 연 창은 검색 경로로
`/usr/bin:/bin:/usr/sbin:/sbin`을 받고, 엔진은 `/opt/homebrew/bin`과
`/usr/local/bin`에 설치된다. DNS 에이전트는 launchd 잡에 이미 그 디렉터리를 담고
있었고 창에는 그런 처리가 없었다.

이제 엔진 명령은 변수, 검색 경로, 설치 디렉터리 순서로 찾는다. 에이전트 잡의 검색
경로도 같은 목록에서 만들므로 둘이 엔진 위치를 다르게 알 수 없다.

검증: `make check`. 그 검색 경로에서 설치 디렉터리의 엔진 찾기, 변수 우선, 에이전트
검색 경로를 검사한다. `/usr/bin:/bin:/usr/sbin:/sbin`으로 시작한 창이 머신 상태를
읽었고, 같은 환경에서 `bay status`도 응답했다.

## 2026-10-01

### 네이티브 주소 이벤트 지원을 설치

`/opt/homebrew/bin/bay`의 CLI는 변경 없는 소스 `95d73d4`를 기록한다.
설치된 CLI로 두 서비스를 교체한 뒤 주소 일치와 HTTPS 사례가 소비 엣지나
DNS 재시작 없이 통과한다. DNS 실행 파일과 메뉴 앱은 갱신하지 않았다.

### 서비스 주소 변경을 구독

Apple 호스트 이름 준비 검사는 DNSServiceGetAddrInfo를 한 번 구독하고
주소 추가·삭제를 처리한다. 완전한 선언 주소 집합이 있어야 준비된다.
성공·오류·기한 만료 시 구독을 취소하고 소켓 작업 종료를 수집한다.
네이티브 주소 콜백이 변경을 보고하므로 반복 조회가 불필요했다.

CLI 빌드 입력에 네이티브 C 소스와 헤더를 포함한다. 추적 테스트는
반복 조회를 허용할 때와 네이티브 입력 변경이 재빌드를 요구하지 않을
때 실패했다. 이벤트·빌드 입력·네이티브 취소 사례가 통과하며 두 서비스
교체 후 실제 주소·HTTPS 검증이 소비 엣지나 DNS 재시작 없이 통과한다.

### 서비스 교체를 검증

교체는 실행 중인 소유 서비스를 중지한 뒤 강제 옵션 없이 제거한다.
중지가 실패하면 원래 오류를 반환하고 제거와 생성을 실행하지 않는다.
이미 중지된 서비스에는 중지 명령을 다시 보내지 않는다. 강제 제거가
애플리케이션의 종료 절차를 건너뛰고 있었다.

Apple 서비스는 의존 서비스 시작과 수명주기 성공 전에 선언된 IPv4·IPv6
주소와 같게 조회되어야 한다. 준비 검사는 최대 60초 안에 정확한 주소 집합을 요구한다. DNS가 제거된 서비스 주소를
반환하는 동안 명령이 성공을 반환하고 있었다. 문서 요청은 재시도하지 않는다.

검증: 추적하는 중지·제거 사례와 의존 서비스 시작 사례는 각각 변경 전에
실패했고 변경 후 통과한다. 주소 일치, 누락되거나 이전 IPv6, 잘못된 선언,
조회 실패 사례가 통과한다. `make check`가 통과한다. 두 서비스 각각의
교체 후 실제 HTTPS 요청이 소비 엣지나 DNS 재시작 없이 통과한다.
변경 없는 소스 `052e567`의 CLI를 설치했다.

## 2026-09-25

### 시작한 서비스의 라우트는 프록시가 연결한 뒤 게시

`up`·`start`·`restart`는 시작한 서비스의 라우트를 실행 중인 프록시에 주기 전에
프록시 컨테이너 안에서 서비스 주소로 `nc -z -w 2`를 250밀리초마다 성공할 때까지
최대 60초 동안 실행하고, 대기를 시작하기 전에 보고하며, 성공하지 않으면 마지막 시도의 오류로 실패한다. 이때
프록시는 이전 설정을 유지한다. 명령이 만드는 프록시는 라우트를 가지고 시작하며,
명령은 같은 시도가 성공한 뒤 반환한다. healthcheck는 서비스 컨테이너 안에서
실행되고 준비 상태 보고는 호스트에서 연결하므로, 둘 다 통과한 뒤에도 프록시를
거친 새 컨테이너로의 첫 요청이 시간 초과될 수 있었다.

검증: `go test ./internal/stack -run
'TestARouteIsPublishedAfterTheProxyConnectsToIt|TestAProxyThatCannotConnectPublishesNothing|TestACreatedProxyIsReturnedAfterItConnects|TestOnlyTheStartedContainersAreConnectedTo'`
(앞의 세 테스트는 변경 전에 실패); `make check`; `go test -count=1 ./...`.
`TestTheFirstRequestToANewContainerIsAnswered`는 `BAY_E2E=1`이 필요하며
실행하지 않았다.

### 초기화는 런타임이 중지를 보고할 때 완료

일회성 서비스의 attach한 start가 반환된 뒤 `up`·`start`·`restart`는 일회성 제한
시간 안에서 런타임이 중지를 보고할 때까지 100밀리초마다 컨테이너를 읽는다. attach한
start는 런타임이 중지 상태를 보고하기 전에 반환될 수 있으며, 한 번만 읽으면 성공한
실행이 "has no stable stopped identity"로 실패했다. 시작한 컨테이너와 다른
컨테이너는 다른 필드마다 하나의 오류로 실패한다: 상태, 생성 시각, 시작 시각, 설정
label, 이미지 digest.

검증: `go test ./internal/stack -run
'TestServiceEngineWaitsForAttachedInitializerToStop|TestServiceEngineNamesEachDifferingStoppedField'`
(두 테스트 모두 변경 전에 실패); `make check`; `go test -count=1 ./...`.

### 프로젝트는 up과 프로젝트 추가만 등록

`install`은 더 이상 작업 디렉터리의 Compose 프로젝트를 등록하지 않고 등록된
프로젝트의 준비를 적용한다. 메뉴바 프로그램은 시작할 때 Compose 파일을 등록하는
`-f`를 더 이상 받지 않으며, 창의 프로젝트 추가가 프로젝트를 등록한다. `status`와
`doctor`는 이전처럼 아무것도 등록하지 않는다. 프로젝트는 `up` 또는 프로젝트
추가로 등록한다.

검증: `go test ./cmd/bay -run DoesNotRegister`(status와 install이 다른
경로로 등록된 항목을 바이트 단위로 그대로 둔다. install 경우는 변경 전에 실패했다)와
`go test ./cmd/baybar -run TestStartTakesNoComposeFileToRegister`(변경 전에
실패했다), `make check`.

### 서비스가 x-bay.domains에 도메인을 나열

서비스는 제공될 모든 도메인을 자신의 `x-bay.domains` 목록에 적는다.
도메인마다 인증서와 같은 컨테이너로 가는 프록시 라우트를 받고, 컨테이너는 요청된
이름을 `Host` 헤더로 받는다. 목록이 없는 서비스는 internal이며 라우트와 인증서가
없다. 목록은 비어 있거나 도메인을 반복하거나 잘못된 이름이나 프로젝트 도메인 밖의
이름을 담을 수 없다. 한 프로젝트의 두 서비스가 같은 도메인을 가지거나 실행 중인
다른 프로젝트의 서비스가 이미 가진 도메인은 두 서비스의 이름을 적은 오류로 거부한다.

컨테이너는 도메인을 쉼표로 구분해 `bay.domain` 라벨에 담으므로 실행 중인
컨테이너는 라우트를 유지한다. `bay status --json`은 서비스마다 `domains`와
`urls` 목록을 보고하고, `status`, `up`, bayd, 메뉴바 창은 모든 도메인을
사용하며 창은 도메인마다 링크와 인증서를 보여 준다. 서비스 식별 정보에 도메인
목록이 들어가므로 이 변경 뒤 프로젝트의 첫 `up`은 컨테이너를 다시 만든다. 볼륨은
유지한다.

검증: `make check`와 `go test -count=1 -v ./...`.

### 시작 절차가 시작된 컨테이너와 상태 검사 시도를 보고

`up`, `start`, `restart`는 서비스 컨테이너가 시작된 뒤, 실패한 각 상태 검사 시도의 경과 시간과 시도 횟수, 서비스가 정상이 된 경과 시간을 한 줄씩 출력한다. 출력을 스트리밍하는 호출자는 대기 진행을 보고, 정상 상태 전에 시작된 컨테이너의 로그에 연결할 수 있다. 상태 검사 의미, 한도, 결과는 바뀌지 않는다.

검증: `go test ./internal/stack -run TestServiceEngineReportsStartAndEveryHealthAttempt`가 가짜 런타임으로 시작, 한도, 실패 시도 두 번, 정상 줄의 순서를 검사한다. `make check`.

## 2026-09-15

### 승인된 기계가 어디 있는지만이 아니라 무엇을 지녔는지 읽는다

피어의 도메인은 기계를 승인할 때 한 번 읽고 다시 읽지 않았다. 알림은 지문, 이름,
주소를 전할 뿐 도메인을 담지 않으므로, 저쪽에서 더하거나 뺀 도메인이 이쪽에 오지
않았다. 새 이름은 제공되지 않았고, 저쪽이 내놓은 이름은 계속 제공됐다.

상주 에이전트가 30초마다 승인된 기계에 무엇을 지녔는지 묻고, 답이 저장된 것과
다르면 프록시 설정을 다시 쓴다. 도메인은 링크 뒤의 문서에서 읽는다. 알림은
네트워크의 누구나 보낼 수 있는 브로드캐스트이기 때문이다.

문서 뒤의 인증기관은 승인한 그것이어야 한다. 다른 인증기관으로 답하는 주소는 다른
기계이며, 그것이 자기 도메인에 대해 말하는 것은 승인된 기계의 것이 아니다. 답하지
않는 기계는 지닌 것을 그대로 유지한다. 도메인을 내놓은 것이 아니라 묻지 못한
것이기 때문이다.

검증: `go test ./internal/stack -run
'TestADomain|TestTheSameDomains|TestAnotherAuthority|TestAPeerThatDoesNotAnswer'`가
도메인 추가, 도메인 제거, 같은 도메인의 다른 순서, 주소의 다른 인증기관, 답하지
않는 기계를 다룬다. `make check` 통과.


### 끝내 응답하지 않는 엔진은 포기한다

엔진 명령에 시간 제한이 없었다. 응답을 멈춘 데몬은 연결은 받고 답하지 않으므로,
명령은 아무것도 반환하지 않고 끝나지도 않았다. 컨테이너 목록을 읽는 모든 것이 함께
멈췄다. 테스트 실행이 그런 호출 하나에서 10분을 앉아 있다가 자신의 시간 제한에
죽었다.

목록 읽기는 15초, 생성과 제거는 10분으로 제한한다. 후자는 이미지를 받거나 프로세스를
멈추는 중일 수 있기 때문이다. 제한 안에 답하지 않는 명령은 응답하지 않는 엔진이며,
컨테이너를 내놓지 않고 다른 엔진을 읽는 것을 막지도 않는다.

명령을 끝내는 것은 그 명령이 시작한 프로세스만 끝내며, 그 프로세스가 또 다른 것을
시작했다면 그것이 출력을 쥐고 있다. 그 출력을 읽으면 아무도 끝내지 않는 프로세스를
기다리게 되므로, 명령을 포기한 1초 뒤에 읽기도 포기한다.

검증: 5분간 잠드는 가짜 엔진에 컨테이너 목록을 요청한다.
`go test ./internal/stack -run TestAnEngineThatNeverAnswersIsGivenUpOn`가 2.9초에
통과하며 명령을 포기하는지, 오류가 그렇게 말하는지, 다른 엔진은 그대로 읽히는지
검사한다. 이 기계의 멈춘 데몬에 대해 10분을 앉아 있던
`TestTheHealthcheckWaitStatesItsBound`가 15.2초에 끝난다. `make check` 통과.

## 2026-09-12

### 실패한 실행은 이유를 말하고, 도는 동안 화면에 있다

`bay up`이 `service mysql stopped during healthcheck`만 내놓았다.
데이터베이스는 크래시했고 자기 로그에 그렇게 적었는데, 도구가 그것을 읽고 버렸다.
프로젝트도 화면에 없었으므로 볼 것도 읽을 것도 없었다. 그 실행은 결국 디스크가
없어서 거부된 것이었고, 런타임은 그렇게 분명히 말했는데 도구가 분류로 뭉갰다.

틀린 규칙 셋을 고친다.

실패는 실패한 서비스의 출력을 보고한다. 마지막 서른 줄로 제한하고 명령의 오류
스트림에만 쓴다. 그것을 감추던 규칙은 아무것도 지키지 못했다. 같은 사람이 같은
터미널에서 `bay logs` 한 번이면 같은 바이트를 보므로, 비밀은 사지 못하고
필요한 순간의 증거만 잃었다.

생성 실패는 분류 뒤에 런타임이 한 말을 보고한다. 런타임은 무엇 때문에 거부했는지를
지목하고 분류는 아무것도 지목하지 못한다. 대신 가리는 것은 좁고 정확하다. 환경 값은
명령줄에 실리므로, 이 명령이 전달한 모든 `KEY=VALUE`를 키와 자리표로 바꾼다. 값이
아니라 쌍을 가린다. 값은 자기가 실려 온 인자의 일부로만 되울려지고, 짧은 값은
그러지 않으면 평범한 단어에 걸린다. 분류기는 런타임이 실제로 쓰는 `does not exist`를
몰랐다.

프로젝트는 그 프로젝트를 위한 어떤 일보다 먼저 등록한다. 등록이 그 프로젝트를 이
기계의 것으로 만들고 창은 등록부를 그리므로, 일이 끝난 뒤 등록하면 긴 실행은 도는
동안 보이지 않고 실패한 실행은 끝내 보이지 않았다. 잠깐을 넘길 수 있는 모든 단계는
이제 시작 전에 서비스와 단계를 말하고, healthcheck 대기는 얼마나 기다릴지 밝힌다.

검증: `make check`. 출력을 싣는 실패, 그 상한, 런타임의 문장, 가리기, 짧은 값처럼
읽히는 평범한 단어, 실패한 `up` 뒤의 등록부, 알림과 그 단계의 순서를 검사한다. 이
기계에서 멈춘 서비스만 보고하던 그 `up`이 이제
`container create failed (resource not found): Error: path
'/Users/maxkwon/orm/.runtime/db-tests' does not exist`를 보고하고, 닿는 서비스와
단계를 차례로 출력한다.


### 주인이 아닌 명령은 아무것도 바꾸기 전에 멈춘다

소유권 검사가 머신 설정 자리에 있었다. 명령이 마지막에 닿는 곳이다. `up`은 볼륨을
만들고 프로젝트를 등록한 뒤에, `down`은 컨테이너를 지운 뒤에 거기 닿는다. 주인이
아닌 상태 디렉터리에서 온 명령은 그 일을 다 하고 나서 멈추며, 이미 바꿔놓은 기계에
대해 실패를 보고하게 된다. 이를 위해 쓴 테스트가 `up`이 이미 등록한 프로젝트로
실패했다.

이제 검사는 up·down·start·stop·restart의 첫 문장이고, 두 앞단의 uninstall에서도
첫 문장이다. `install`은 예외다. 설정을 넘겨받는 것이 그 명령의 목적이다.

검증: `make check`. 그 다섯이 아무것도 등록하지 않고 멈추는지 검사한다. 이 기계에서
두 번째 상태 디렉터리로 `sync`·`uninstall`·`down`을 돌리자 각각 주인을 말하며
멈췄고 기계도 그 디렉터리도 그대로였다.

### 머신 설정에는 주인이 하나고, 프로젝트 실행은 아무것도 거두지 않는다

`bay -state <다른곳> up`이 `/etc/resolver/devel`과
`/etc/resolver/staging`을 제거하겠다며 관리자 권한을 요구했다. 그 항목들은 이
기계를 설정한 상태 디렉터리의 것이다. 다른 디렉터리는 그 도메인을 위임하지
않았고, 코드는 파일 내용이 자기가 쓰는 형식과 같다는 이유로 제거해도 되는
항목이라고 판단했다. 암호를 줬다면 한 상태 디렉터리가 다른 쪽의 위임을 거뒀을
것이다.

리졸버 항목, DNS 에이전트, 프록시는 기계의 것이고 각각 하나씩이다. 항목은
도메인으로 식별되고, 에이전트는 도메인 목록 하나를 든 launchd 잡 하나이며,
프록시는 엔진마다 컨테이너 이름 하나다. `-state`는 인증기관·인증서·프로젝트
등록부가 있는 디렉터리를 고르는 것이고, 나눌 수 없는 것을 나누지 않는다. 머신
설정의 주인은 상태 디렉터리 하나이고, 그 주인은 등록된 에이전트가 실행 중인 상태
디렉터리다. 도메인에 답하는 그 프로그램에서 읽으므로 따로 적어둔 기록과
어긋날 수 없다. 다른 상태 디렉터리에서 온 명령은 셋 중 어느 것도 쓰지 않는다.
멈추고 주인을 말한다. 주인을 가져가는 명령은 `bay install` 하나다.

위임 해제는 이제 이름으로 요청한다. `bay domain remove`가 리졸버 항목을
제거한다. 이전에는 제거하지 않았다. `uninstall`은 전부 제거한다. 그 밖의 어떤
명령도 제거하지 않는다. 이 기계의 어느 도메인도 덮지 않는 항목은 `doctor`가
보고하고 그대로 둔다. 이 도구가 썼다고 보일 수 없는 `/etc` 아래 파일은 이 도구가
지울 것이 아니다.

설정 중 사람 앞에 무언가를 띄우는 단계는 둘이고 서로 다른 것을 묻는다.
`/etc/resolver` 쓰기는 관리자 권한이 필요하다. 인증기관 신뢰는 시스템 도메인이 아니라
로그인 키체인에 쓰므로 root가 필요 없지만, macOS가 자기 신뢰 설정 대화상자를 띄우고
로그인 암호를 기다린다. 둘 다 키보드 앞에 사람이 없는 프로그램은 답할 수 없으므로,
남은 단계마다 둘 중 어느 것을 묻는지 밝힌다.

둘 다 명령마다가 아니다. 도메인은 한 번 위임하고 인증기관은 한 번 신뢰한다. 이미
위임된 도메인 아래에서 이미 신뢰된 인증기관을 쓰는 상태 디렉터리로 제공되는
프로젝트는 화면에 아무것도 뜨지 않는다.

검증: `make check`. 주인 읽기, 다른 상태 디렉터리 거절, 이름으로 거두기, 주인
없는 항목 그대로 두기를 검사한다. 이 기계에서 `install`이 화면에 아무것도 띄우지
않고 주인을 가져가며 에이전트를 다시 등록했다. 모든 도메인이 이미 위임되어 있고
인증기관도 이미 신뢰되어 있었기 때문이다. 버릴 인증서로 신뢰 명령을 실행하자
신뢰되지 않았을 때 뜨는 대화상자가 나타났다. 두 번째 상태 디렉터리에서 `doctor`가 두 항목을
제거 제안 대신 그대로 둔다고 보고했으며, 그 디렉터리에서 `sync`는 exit 1로 멈추고
주인을 말했고 아무것도 바꾸지 않았다.

## 2026-09-10

### 피어를 실제로 닿은 주소로 승인한다

기계를 승인할 때 그 기계가 자신에 대해 말하는 주소를 저장했다. 그것은 그 기계가
속한 네트워크의 주소다. 라우터 뒤의 기계는 그 네트워크 안에서만 닿는 주소를
말하므로, 공인 주소로 승인해도 사설 주소가 저장되고 이후 모든 요청이 닿지 않았다.

명령줄과 창 모두 실제로 닿은 주소를 저장한다. 기계가 자신에 대해 말하는 주소는
자기 네트워크에 알리는 값으로 남으며, 그 네트워크에서 옮겨간 기계를 따라가는 데
쓰인다.

링크 기록을 읽는 방법을 명령 하나로 적었다. 상대가 chunked로 보내는 기계는 길이를
선언하지 않으므로 요청 옆의 오류가 유실을 알리기 때문이다.

검증: `go test ./internal/stack -run TestAnApprovedPeerKeepsTheAddressItWasReachedAt`가
203.0.113.5:8443으로 승인한 피어가 그 주소로 저장되는지 검사한다. `make check` 통과.


### 이름을 제공하는 포트에서 프록시가 응답하는지 확인한다

경로를 만드는 명령은 프록시가 health 엔드포인트에 응답하면 반환했다. 그것은 80번
포트다. 모든 이름은 443에서 제공하는데, 프록시 컨테이너를 교체한 뒤 엔진의 443
포워딩이 연결 즉시 리셋되는 상태로 남았고 80과 피어 링크는 정상 응답했다. 명령은
성공을 보고했고 어떤 이름에도 닿을 수 없었다.

이제 호스트가 프록시에 닿는 주소에서 이름을 제공하는 포트를 열고 핸드셰이크를
완료한다. 컨테이너는 실행 중인데 그 포트가 응답하지 않으면 컨테이너를 다시
시작한다. 그것이 엔진의 포워딩을 다시 만드는 동작이며, 그 뒤 포트를 다시 묻는다.
명령은 이름에 닿을 수 있게 된 뒤에만 반환한다.

검증: 프록시 컨테이너를 제거하고 `bay sync`를 실행했다. 변경 전에는 같은
순서에서 `https://127.0.0.1/`이 연결을 리셋하는 동안
`http://127.0.0.1/__bay/health`와 8443의 피어 링크는 모두 응답했고
`docker restart`로만 복구됐다. 변경 후에는 명령이 반환한 직후 사이트가 200과
11250바이트로 응답했다. `make check` 통과.


### 피어의 도메인만 답하는 기계에서 사라진 프록시를 보고한다

화면은 이 기계 자신의 컨테이너가 실행 중이고 경로를 지닐 때만 사라진 프록시를
보고했다. 승인된 피어가 있고 자기 컨테이너는 없는 기계도 같은 프록시로 그 피어의
도메인을 답하는데, 프록시가 사라져도 아무 말도 하지 않았다. 이름 둘이 답을 못 받는
동안 대시보드는 "실행 중인 것 없음"이라고 표시했다.

이제 기계가 답하는 것을 자기 라우트 도메인과 승인된 피어가 지닌 도메인의 합으로
센다. 답할 것이 있는데 프록시가 실행 중이 아니거나 응답하지 않으면 보고하고, 다시
시작하는 동작을 함께 제공한다.

배너는 컨테이너가 실행 중이라고 말하고 닿을 수 없는 것을 경로라고 불렀다. 피어의
도메인만 제공하는 기계에서는 둘 다 사실이 아니다. 자기 컨테이너는 실행 중이 아니고
그 이름들은 피어의 것이다. 이제 닿을 수 없는 이름이 몇 개인지와 그 밖에 바뀐 것이
없음을 말한다.

검증: 프록시를 제거하자 창이 결함을 표시했다. `연결 불가 · 서비스 0개 실행 중 ·
프록시가 응답하지 않음`이라는 빨간 판정 아래 `진단 실행`과 `프록시 다시 시작`을
제공하는 배너가 나왔다. 변경 전에는 같은 기계에서 아무 결함도 표시하지 않았다.
`make check` 통과.


### 링크 기록의 모든 값에 단위를 붙인다

기록이 바이트 수와 초를 나란히 두고 어느 쪽도 단위를 밝히지 않았다. 그래서
`sent=3893`과 `response=0.055`를 무엇을 재는 값인지로 구분할 수 없었다. 이제 각
값이 단위를 지닌다. 바이트 수 뒤에는 `bytes`가 붙고 초는 `s`로 끝난다.

검증: 링크를 지난 요청이 `link sent=11250 bytes declared=- bytes framed=11270 bytes
wire=11502 bytes connect=0.023s header=0.040s response=0.040s`를 기록했다.
`BAY_E2E=1 go test ./internal/stack -run 'TestAPeerRoute.*ActualRuntime'`와
`make check` 통과.


### 런타임 테스트를 기계가 가진 엔진에서 실행한다

런타임 테스트 넷이 `container`를 직접 지정해서, Docker는 있고 Apple `container`는
없는 기계에서 실행이나 건너뛰기가 아니라 명령을 찾지 못해 실패했다. 이제 프로그램의
나머지와 같이 서비스를 만드는 엔진에 묻는다.

검증: Docker가 있고 Apple `container`가 없는 기계에서
`BAY_E2E=1 go test ./internal/stack -run ActualRuntime`가 통과한다.
변경 전에는 `TestPeerConfigurationIsAcceptedByNginxActualRuntime`가
`exec: "container": executable file not found in $PATH`로 실패했다.
`make check` 통과.


### 같은 기준의 두 수로 본문 유실을 판정한다

기록이 상대에게서 읽은 길이와 클라이언트로 보낸 길이를 나란히 두었는데, 이 둘은
온전한 응답에서도 항상 다르다. 앞의 것은 chunked 틀을 포함해 세고 뒤의 것은 세지
않기 때문이다. 그래서 온전한 응답이 유실처럼 읽혔다.

이제 상대가 선언한 길이를 기록한다. 그 값은 보낸 길이와 같이 본문 바이트로 세어지므로
바이트를 잃었을 때만 둘이 다르다. 상대가 길이를 선언하지 않으면 선언 길이가 비고,
프록시가 그 요청에 대해 쓰는 오류가 유일한 신호다. 요청 자체의 완료 여부는 더 이상
기록하지 않는다. nginx는 잘린 응답을 완료된 요청으로 보고해서 사실과 반대로 말했다.

런타임 테스트 둘이 링크의 양쪽 끝을 세우고 각각 이 프로그램의 설정으로 실행한다.
하나는 프록시 버퍼보다 큰 본문을 요청해 선언 길이와 도착 길이가 같은 수인지 검사한다.
다른 하나는 100000바이트를 선언하고 5바이트를 보내는 상대를 세워, 기록이 두 수를 모두
남기는지와 프록시가 그 요청에 오류를 쓰는지 검사한다. 둘 다 무엇을 언제 멈추는지에
의존하지 않는다.

검증: `BAY_E2E=1 go test ./internal/stack -run
'TestAPeerRoute.*ActualRuntime'` 통과. 길이를 선언하지 않는 이 기계의 피어에 대해
온전한 응답이 `sent=3893 declared=-`를 기록했다. 응답을 자르는 테스트에서 기록은
`sent=5 declared=100000`이었고 같은 요청에 대해
`upstream prematurely closed connection`이 함께 남았다. `make check` 통과.


### 프록시가 링크 기록을 받아들이지 않았음을 알린다

스위치가 설정을 쓰고 자신을 켜진 상태로 그린 뒤, 화면 뒤에서 설정을 적용하면서
결과를 읽지 않았다. 그것이 실패하면 스위치는 켜진 것으로 보이는데 프록시는 이전
설정을 그대로 실행하므로, 기록을 요청했는데 아무것도 쓰이지 않고 그에 대해 아무
말도 없었다.

이제 결과를 읽는다. 프록시가 기록을 받아들이지 않으면 설정은 프록시가 실행 중인
값으로 돌아가고 스위치가 그것을 따르며 실패를 화면에 알린다.

검증: `make check` 통과. 기록을 켠 상태에서 링크를 지난 요청이
`link 200 host=polyspec.test uri=/comparison.css declared=3905 received=4130
sent=3893`을 기록했다.


### 링크 기록 스위치를 화면을 다시 그리지 않고 응답한다

기록을 켜는 것이 동작으로 실행됐다. 모든 버튼을 비활성으로 화면을 한 번 그리고,
프록시 설정을 쓰고 다시 읽은 뒤, 기계 상태를 새로 읽어 화면을 또 그렸다. 그 사이에
바뀐 것은 스위치뿐이다.

스위치는 이제 화면에서 즉시 응답하고 설정 쓰기와 다시 읽기는 그 뒤에서 진행하므로
다른 것은 움직이지 않는다. 프록시는 기록을 설정에서 읽고 설정을 다시 읽지 않고
바꿀 방법이 없으므로 그 부분은 남는다.

검증: `make check` 통과.


### 피어 링크를 지나는 것을 기계가 요청할 때 기록한다

상대 기계가 도중에 끊은 응답도 200으로 답하므로, 그중 얼마가 클라이언트에
도착했는지 프록시 출력에 남지 않았다. 그런 응답 하나의 원인을 규명하지 못했다.
어떤 변경 뒤에 같은 요청이 60회 연속 온전히 도착했고 그 전에는 20회 중 1회 짧게
도착했으며, 측정 시점마다 비율이 달라졌다. 그래서 그 변경이 원인이라고 증명되지
않았다.

설정에 링크를 지나는 요청마다 한 줄을 기록하는 스위치를 두었다. 상대 기계가 보낸
길이, 상대에게서 받은 바이트 수, 클라이언트에 도착한 길이, 연결까지와 첫 헤더까지와
응답 끝까지의 시간을 남긴다. 짧게 보낸 것이 느낌이 아니라 기록의 숫자가 된다. 켜기
전에는 꺼져 있다. 링크를 지나는 모든 요청에 한 줄을 쓰기 때문이다.

`internal/stack/peer_body_runtime_e2e_test.go`는 링크의 양쪽 끝을 세우고 각각 이
프로그램의 설정으로 실행한 뒤, 프록시 버퍼보다 큰 본문을 다섯 번 요청한다. 결함을
재현하지는 못하므로 동작을 지킬 뿐 설명하지는 않는다.

검증: 스위치를 켠 상태에서 링크를 지난 요청이
`link 200 host=polyspec.test declared=327568 received=327800 sent=327197`을
기록했다. 선언된 길이와 보낸 길이의 차이는 온전한 응답의 chunk 프레이밍이다.
`make check` 통과, `BAY_E2E=1`로 런타임 테스트 통과.


### 피어로 가는 라우트에서 TLS 세션을 재사용하지 않는다

피어로 가는 라우트는 클라이언트 인증서를 제시한다. 그런 연결에서 TLS 세션을
재사용하면 피어가 본문을 완료하기 전에 응답을 종료했고, 프록시는 받은 만큼과 함께
200을 반환했다. 페이지는 마크업만 불러오고 스크립트를 하나도 불러오지 못했다.

앞선 변경이 같은 요청에서 `Connection: close`를 제거했고 측정하는 동안 절단이
멈췄다. 그러나 다시 나타났다. 그 헤더는 잘못 보내던 것이라 제거는 그대로 두지만
원인은 아니었다.

검증: 피어 링크에 대고 측정했다. 세션을 재사용할 때 같은 327197바이트 파일이
163431, 179799, 163447바이트로 도착했고 매번 200이었으며, 같은 시각 피어에 직접
보낸 요청은 매번 온전히 도착했다. 재사용하지 않으면 다섯 번 연속 온전히 도착했고
프록시는 상류 오류를 남기지 않았다. `make check` 통과.

## 2026-09-09

### 창을 측정하는 도구를 남긴다

창의 간격을 한 세션용으로 작성한 프로그램으로 측정하고 임시 디렉터리에 두었다.
세션이 끝나면 지워지므로 다음 레이아웃 변경은 다시 눈대중으로 판단하게 된다.

`tools/window.swift`는 창을 식별자로 캡처하므로 창이 앞에 있지 않아도 되며, 글자를
담은 픽셀 행마다 그 중심에서 다음 중심까지의 거리를 보고한다. `make window`가 이를
실행하고 `AGENTS.md`가 사용법을 기술한다.

검증: `make window OUT=/tmp/w.png REGION="262 895 205 390"`가 실행 중인 창을
캡처하고 피어 카드의 행이 23.0, 24.5, 24.0, 24.0 떨어져 있으며 선이 있는 경계는
35.0임을 보고했다. `make check` 통과.
### 카드의 행 간격을 여백 하나로 맞추고 화면에서 측정한다

피어 카드는 경계마다 다른 공간을 두었다. 행 중심 사이가 23, 32, 27, 24였고,
주소와 지문 사이는 카드 가장자리의 두 배가 넘었다.

부분을 명명하는 카드는 이제 여백을 한 번 정하고 나머지를 거기서 끌어낸다. 선이
없는 두 행은 여백 하나만큼 떨어지며 이는 카드 가장자리의 공간과 같다. 선은 부분이
시작하는 자리를 표시하므로 선이 있는 경계는 선 양쪽에 여백 하나씩을 둔다. 그런
카드의 모든 행은 여백에 한 줄을 더한 높이로 고정한다. 라벨과 값을 감싸는 스택이
글자가 필요한 것보다 높이를 더 잡아, 여백을 어떻게 주든 그 경계만 넓게 남았기
때문이다.

평범한 목록인 카드는 바뀌지 않는다. 그런 카드는 모든 경계에 선이 있고 행은 쓰던
여백을 그대로 쓴다.

검증: 화면에서 측정했다. 창을 열어 창 식별자로 캡처하고 글자가 있는 픽셀 행을
읽었다. 변경 전 중심 사이 23, 32, 27, 24. 변경 후 선이 없는 모든 경계가 24이고
선이 있는 경계는 35로 여백 하나와 선 두께만큼 넓다. `make check` 통과.
### 카드의 부분이 시작하는 자리에만 구분선을 그린다

창이 카드의 모든 행 사이에 구분선을 그렸다. 제목, 값 둘, 목록으로 이루어진 카드가
구분 없는 하나의 목록으로 읽혔다.

부분을 명명하는 행을 가진 카드는 부분이 시작하는 자리, 즉 라벨 행 위에만 구분선을
그린다. 평범한 목록인 카드는 여전히 모든 행 사이에 그린다. 그 경우 행이 곧
항목이기 때문이다.

피어의 지문 아래에서 지문이 무엇인지 설명하던 줄은 제거했다. 기계가 아니라 설계를
설명하는 문장이었다.

검증: `make check` 통과. 창은 보지 못했다. 메뉴바 애플리케이션이라 스크립트로 앞에
나오지 않는다.
### 승인된 기계와 그 도메인을 하나의 섹션으로 표시한다

화면은 승인된 기계의 이름만 표시하고 그 기계가 제공하는 도메인을 표시하지 않았다.
명령줄은 그것을 출력한다. 승인된 기계가 여럿이면 어느 기계가 어느 도메인을
제공하는지 읽을 수 없었다.

기계 하나가 카드 하나다. 카드의 첫 행이 기계를 표시하고 제거 동작을 가지며, 다음
두 행이 주소와 지문을, 라벨 행이 뒤따르는 도메인 개수를 표시한다.
행은 그 기계가 제공하는 도메인이며 각 행은 그 주소를 연다. 도메인을 제공하지 않는
기계는 그렇게 표시한다.

기계와 도메인을 한 목록에 담으면 도메인의 소속을 열 하나로 표시하게 되고, 그것은
행의 또 다른 속성으로 읽힌다. 섹션이 소속을 표시한다. 제거는 기계에 대한 동작이고
열기는 도메인에 대한 동작이며, 이제 각각 대상 위에 있다.

도메인 행은 도메인을 표시하고 클릭하면 그 주소를 연다. 이름 옆에 주소를 함께
표시하던 것은 같은 것을 두 번 적는 것이었다.

주소와 지문을 처음에는 섹션의 `detail` 필드에 넣었다. 창은 그 필드를 읽지 않으므로
둘 다 사라졌고 화면은 기계 이름만 표시했다. 그 필드는 어디에서도 읽지 않으므로
제거했다.

창은 두 가지 행을 더 그린다. `title`은 카드가 무엇을 나타내는지 표시하고 그것에
대한 동작을 가지며, `label`은 그 아래 행들을 명명한다.

검증: `go test ./cmd/baybar -run 'TestThePeersScreen|TestAPeerSection'`가
승인된 기계 둘과 그 사이의 도메인 셋을 다루고, 각 도메인이 그것을 제공하는 기계의
섹션 안에 있는지 검사하며, 주소와 지문이 창이 그리는 행에 있는지 검사한다.
`make check` 통과.
### 인증서 사용 판정에 피어 도메인을 포함한다

이 기계는 승인된 피어의 도메인을 자신이 발급한 인증서로 제공한다. 인증서 화면은
라우트와 프로젝트만으로 이름의 사용 여부를 판정했고, 그래서 그 인증서들을 미사용으로
보고하고 제거를 제안했다. 제거하면 그 이름을 제공하는 데 필요한 인증서가 사라진다.

승인된 피어의 도메인을 판정에 포함한다. 어떤 라우트도 프로젝트도 피어도 요구하지
않는 이름은 그대로 미사용으로 보고한다.

검증: `bay status --json`이 제공 중인 polyspec.test와 registry.soksak.test를
orphaned로 보고했다. 변경 후 둘 다 orphaned가 아니고, 아무것도 요구하지 않는 이름은
그대로 orphaned다. `make check` 통과.

### 신뢰되지 않은 인증기관을 보고한다

어떤 키체인도 인증기관을 갖고 있지 않은데 setup은 남은 단계가 없다고 보고했다.
그래서 브라우저가 이 기계의 모든 이름을 거부했고 이유를 알리는 메시지가 없었다.
원인은 결함 두 개다.

검사가 인증기관을 자기 자신으로 검증했다. 자기서명 인증서는 자신이 루트이므로 그
검사는 키체인 없이도 성공했고, 방금 생성한 인증기관에도 true를 반환했다. 이제
키체인이 그 인증서를 갖고 있는지 검사하며, 주체 이름이 아니라 인증서 바이트를
비교한다. 교체된 인증기관은 교체한 것과 주체 이름이 같기 때문이다.

인증기관을 추가하는 명령이 키체인을 지정하지 않았다. 지정하지 않으면 0을 반환하고
아무것도 추가하지 않는다. 이제 로그인 키체인을 지정하며, 추가와 제거 모두 반환값
대신 결과를 다시 읽는다.

검증: 어떤 키체인도 인증기관을 갖고 있지 않았고 그것이 발급한 인증서는
`security verify-cert -p ssl`에 실패했는데 setup은 남은 단계가 없다고 보고했다.
변경 후 setup이 신뢰 단계를 보고했고, 실행했고, 인증서가 검증됐다. 두 번 호출하면
같은 결과가 나오고, setup을 다시 실행하면 남은 단계가 없다고 보고한다.
`make check` 통과.

### 프록시 요청에서 Connection: close 를 제거한다

프로토콜 업그레이드가 아닌 요청이 상류에 `Connection: close`를 보냈고, 상류는 본문을
완료하기 전에 응답을 종료했다. 프록시는 content length보다 짧은 본문과 함께 상태
200을 반환했고, 그래서 브라우저는 성공으로 통보받은 요청에서 프로토콜 오류를
보고했으며 페이지는 마크업만 불러오고 스크립트를 하나도 불러오지 못했다.

업그레이드 요청은 그대로 `Connection: upgrade`를 보낸다. 나머지 요청은 Connection
헤더를 보내지 않는다. 매핑 값이 비면 nginx가 그 헤더를 생략한다.

검증: 피어 링크에 대해 측정했다. 변경 후 327197바이트 파일을 네 번 모두 완전히
수신했다. 변경 전에는 같은 요청이 163440, 212536, 310744, 114351바이트를 반환했고
매번 상태는 200이었다. 이 프록시가 추가하는 헤더 네 개를 각각 단독으로 보냈고
`Connection: close`만 절단을 재현했다. `make check` 통과.

### Docker 프록시를 127.0.0.1에 게시한다

Docker 프록시가 127.0.0.2에 게시했다. macOS는 lo0에 그 주소를 할당하지 않으므로
Docker가 컨테이너 생성을 거부했고 피어 링크 열기가 실패했다. 그 주소를 추가하려면
root가 필요하며, root는 `/etc/resolver`에만 사용한다.

같은 명령을 중단시킨 결함이 둘 더 있었다. 자기 라우트가 없는 설정이 주소 없는
resolver 지시자를 작성했고, nginx는 그 상태로 시작하지 않는다. 피어 링크가 열려
있고 라우트가 없는 기계가 프록시를 실행하는 대신 중지했고, 그래서 기계 승인에
사용하는 문서를 제공하는 것이 없었다. resolver 지시자는 주소가 있을 때만 작성하며,
링크를 게시하는 프록시는 라우트가 없어도 실행한다.

Docker 프록시는 네트워크 게이트웨이 대신 Docker의 resolver 주소를 사용한다. Docker는
게이트웨이에서 응답하지 않는다.

검증: 게시 주소를 바인딩하는 테스트가 첫 결함을 재현한다. `bay peer open`이
192.168.0.10:8443에 링크가 열렸다고 보고했고, 프록시가 80과 443을 127.0.0.1에,
8443을 네트워크에 게시하고 실행 중이었다. 네트워크 주소로 요청하면 이 기계의 이름,
주소, 인증기관이 담긴 문서를 반환한다. `make check` 통과.

### 설정을 저장하기 전에 피어 링크를 거부한다

프록시 설정은 두 엔진을 실행하는 기계에서 피어 링크를 거부한다. `peer open`과 창의
버튼은 설정을 먼저 저장하고 거부를 나중에 만났고, 그래서 거부된 기계가 링크를 열린
것으로 기록한 채 남았으며 이후 모든 설정 쓰기가 같은 이유로 실패했다. 설정 파일을
직접 편집하는 것 외에 복구 방법이 없었다.

거부는 함수 하나이며 설정과 프록시 설정 양쪽이 호출한다. 여는 동작은 쓰기 전에
호출한다. 닫는 동작은 거부하지 않는다. 링크가 열린 상태에서 두 번째 엔진이 생긴
기계도 닫을 수 있다.

검증: `make check`. 엔진 하나, 둘, 없음, 두 번째 엔진이 생긴 뒤 닫기를 포함한다.
변경 전에는 두 개가 실패한다. 두 엔진에서 여는 것이 허용되고 설정이 켜진 채 남는다.

### 창에 피어를 표시한다

창은 이 기계가 도메인에 도달하는 데 사용하는 기계를 나열한다. 화면은 링크 주소,
알림 주기, 승인된 기계와 그 도메인, 이 네트워크에서 알리고 있는 기계를 표시한다.
링크는 같은 화면에서 열고 닫는다.

검증: 창에서 링크를 열고 닫았고 명령줄이 같은 상태를 보고했다.

### Apple container 와 Docker 를 함께 지원한다

기계는 Apple `container`와 Docker를 동시에 실행하고 둘 다 제공한다. 어느 쪽도
우선하지 않는다. 한쪽 엔진의 명령만 있고 다른 쪽은 없는 기계에서 `bay status`가
이제 실행된다. 변경 전에는 시작하지 않았다.

컨테이너는 존재하는 모든 엔진에서 읽어 하나의 목록으로 병합하며, 각 컨테이너는 자기
엔진을 기록한다. 없는 엔진이나 데몬이 응답하지 않는 엔진은 컨테이너를 반환하지 않고
오류를 발생시키지 않는다.

엔진마다 자기 프록시를 실행한다. 컨테이너는 자기 엔진의 네트워크에서만 도달 가능하기
때문이다. 호스트는 Apple 프록시에 그 프록시의 주소로, Docker 프록시에 127.0.0.1에
게시한 80과 443으로 도달한다. 서비스 자신의 포트는 양쪽 모두 게시하지 않는다.
`bayd`는 요청된 이름의 엔진에 해당하는 프록시 주소를 반환한다.

도메인은 기계당 한 번 점유한다. 점유는 어느 프록시를 설정하기 전에 모든 엔진의
컨테이너를 대상으로 확정한다.

DNS 서버의 서비스별 모드를 제거했다. 컨테이너 자신의 주소를 반환하는 방식이며 호스트가
경로를 가진 네트워크를 요구하므로 Docker 컨테이너를 제공할 수 없었다. `-proxy`와
`-aliases` 플래그도 함께 제거했고 에이전트는 그것들 없이 등록한다.

`docs/spec/architecture.md`는 각 규칙이 의존하는 조건과 함께 번호가 붙은 규칙 열 개를
기술한다. `AGENTS.md`는 코드가 각 규칙을 어디에서 구현하는지와 구현하지 않은 것이
무엇인지 기술한다.

피어 링크는 옮기지 않았다. 포트 하나를 사용하며 규칙 8이 그것을 두 엔진에 도달하는
호스트 프로세스에 둔다. 그 프로세스를 만들기 전까지, 두 엔진이 있는 기계에서 링크를
여는 것은 한 엔진에서 제공하고 다른 엔진의 도메인을 누락하는 대신 오류를 반환한다.

검증: `make check` 통과, `go test -race ./internal/stack` 4.430초 통과. 단위 테스트는
`docker inspect` 파싱, 네트워크가 여럿인 컨테이너의 주소 선택, 응답하지 않는 엔진,
엔진 없음, 엔진을 넘은 도메인 점유를 다룬다. 실행 중인 Docker 데몬에 대해
`stack.List`가 이 기계의 컨테이너 다섯 개를 읽었고 각각 자기 엔진을 기록했으며,
실행 중인 셋은 브리지 주소를, 종료된 둘은 주소 없음을 반환했다. `docker inspect`가
반환하는 필드 이름을 파서와 비교했다. Apple `container`에서는 아무것도 검증하지
않았다. 이 기계에 없다.

### 기계를 찾는 길은 둘, 증명하는 길은 하나

링크가 열린 기계는 1.5초마다 자신을 네트워크에 알린다. `bay peer find`가
들리는 것을 나열하고 `peer add <이름>`이 알린 이름으로 승인한다. 주소를 직접
알려주는 것이 다른 한 길이고, 서브넷이 다르거나 네트워크가 브로드캐스트를 거르는
곳에서 되는 쪽이다. 둘은 서로를 대체하지 않는다. 알림이 주는 것은 어디를 볼지뿐이고
인증기관은 기계에서 직접 읽으므로, 기계를 증명하는 것은 여전히 인증서 하나다.

알림은 멀티캐스트가 아니라 브로드캐스트다. 액세스 포인트가 클라이언트 사이의
멀티캐스트를 걸러내면서 서브넷 브로드캐스트는 흘려보내는 경우가 흔하기 때문이다.

상주 에이전트는 다른 주소로 돌아온 피어를 따라간다. 네트워크가 다른 주소를 내주면
그렇게 된다. 알림은 피어의 인증기관 지문으로 대조하고 주소만 가져온다.

가는 길에 결함 둘을 찾았다. launchd에 등록된 에이전트가 어떤 프로그램을 실행하든
current로 보고되어, 옮겨진 복사본에서 등록된 잡이 계속 통과했다. 이제 프로그램도
판단에 들어가고, 재등록할 때 launchd가 아직 붙들고 있는 잡이 사라질 때까지 기다린
뒤 새로 등록한다. 둘 다 이 기계에서 드러났다. 에이전트가 하루 동안 임시 빌드에서
돌고 있었다.

검증: `make check`. 알림 수신, 닫힌 링크의 침묵, 만료, 같은 기계의 재알림을 검사한다.
이 기계에서 `peer find`가 자기 알림을 나열했고 `peer add max`가 이름으로 승인했다.
이후 링크가 승인된 인증서에 200으로 제공했고, `peer remove`와 `peer close` 뒤에도
이 기계의 사이트는 계속 답했다.

### bay peer

한 네트워크의 두 기계가 각각 bay을 실행하며 서로의 도메인에 닿는다. 어느
쪽의 네트워크 설정도 바뀌지 않는다.

`bay peer open`이 공개한 포트에서 링크를 답하고 상대 기계에 알려줄 주소를
출력한다. `peer add <주소>`는 그 기계가 자신에 대해 말하는 것을 읽고, 말한
인증기관을 실제로 갖고 있는지 확인하고, 지문을 출력한 뒤 승인 여부를 묻는다.
`peer`는 승인된 것과 링크 상태를 보여준다. `peer remove`가 승인을 거두고 `peer
close`가 답을 멈춘다.

피어는 그 인증기관이다. 인증기관의 지문으로 저장되고, 이름과 주소는 바뀌어도 다른
피어가 되지 않는 속성이다.

공개한 포트는 출발지 주소를 유지하지 않으므로 주소로 기계를 알아보지 않는다. 각
기계는 자기 인증기관이 발급한 인증서를 제시하고, 링크는 승인된 인증기관의 인증서를
가진 쪽에만 제공한다. 키체인에는 아무것도 추가하지 않는다. 승인된 인증기관은
프록시가 읽는 파일이다.

피어의 도메인은 여기서 답한다. 이 기계가 자기 인증기관으로 발급한 인증서로 받고
그 피어의 링크로 전달한다. 그래서 브라우저는 자기 기계가 이미 신뢰하는 인증기관의
인증서를 받는다. 이 기계가 제공하는 이름은 언제나 이 기계의 것이다.

검증: `make check`. 이 기계에서 링크를 열어 이름·주소·도메인·인증기관이 담긴 문서를
확인했고, `peer add`가 기계가 말한 인증기관으로 인증서를 검증한 뒤 승인했다. 이후
링크의 제공 중인 이름이 클라이언트 인증서 없이는 400, 있으면 200이었다. `peer
remove`와 `peer close`가 포트를 풀었고 이 기계의 사이트는 계속 200을 반환했다.

### 미사용 인증서는 별도 목록이다

무언가 요구하는 인증서와 남겨진 인증서는 답이 다르다. 하나는 재발급하고 다른
하나는 제거한다. 인증서 화면이 둘을 나누어 보여준다. 미사용 머리글에 `전체 N개
제거`가 있고 각 행에 `제거`가 있으며, 제거 전에 묻는다. 제거한 인증서는 돌아오지
않는다.

일괄 제거가 무엇을 지울지는 질문을 거쳐 넘기지 않고 실행하는 순간 머신에서 다시
읽는다. 질문이 열려 있는 동안 사용되기 시작한 이름은 남긴다.

검증: `make check`. 일괄 동작이 고르는 대상을 검사한다. 이 머신에서 머리글이
`전체 17개 제거`로 표시됐고, 확인 창을 화면으로 읽은 뒤 취소했으므로 인증서
24개는 그대로다.

### 인증서는 그 이름을 아무도 요구하지 않을 때만 미사용이다

라우트가 그 이름을 제공하지 않으면 인증서를 미사용으로 불렀다. 프로젝트의 실행
여부는 인증서를 미사용으로 만드는 근거가 아니다. 정지된 프로젝트는 아무것도
제공하지 않으므로 그 인증서가 전부 미사용으로 보고되고 제거가 제안됐다. 다음
시작에 필요한 것들이다. 이제 라우트도, 등록된 프로젝트도 그 이름을 요구하지 않을
때만 미사용이다. 파일이 사라진 프로젝트가 남기는 것이 그것이다.

파일은 그대로 있는데 읽지 못한 프로젝트는 무엇을 요구하는지 알 수 없다. 그동안은
아무것도 미사용으로 부르지 않는다. 그러지 않으면 그 프로젝트에 필요한 인증서를
제거하라는 버튼을 내놓게 된다.

검증: `make check`. 라우트 없는 선언된 도메인, 제공 중인 도메인, 대소문자, 파일은
있으나 읽지 못한 프로젝트를 검사한다. 이 머신에서 모든 프로젝트가 정지한 상태로
변경 전에는 `api.platform3.test`를 미사용이라 불렀고, 이제는 그것을 선언한
프로젝트를 표시하며 어떤 프로젝트도 요구하지 않는 이름 14개만 표시가 남는다.

### 목록의 긴 이름은 구분되는 부분을 남긴다

인증서 목록은 이름마다 고정 열을 준다. 접두사가 같고 해시만 다른 이름들은 그
열에서 모두 같게 읽힌다. 인증서 24개 중 14개가 `console.platform-nat…`으로
보였다. 이름이 행을 구분하는 것이므로, 이제 행에서 달리 쓰지 않는 폭을 이름이
가져가고, 들어가지 않는 부분은 끝이 아니라 가운데를 버린다. 옆의 짧은 문구는
눌리지 않고 제 폭을 지킨다.

검증: 이름이 온전히 들어가지 않는 창 너비에서 목록을 화면으로 읽었다. 각 행이
`console.plat…605595.test`로 읽히고, 모든 행이 `사용하는 경로 없음`을 온전히
표시한다.

## 2026-09-08

### bay sync

프록시 설정은 컨테이너를 시작하거나 정지시키는 명령만 쓴다. 그래서 설정을 쓰는
방식이 바뀌어도 이미 컨테이너가 돌고 있는 머신에는 재시작 없이 반영할 수 없었다.
`bay sync`는 실행 중인 컨테이너에서 설정을 다시 쓰고, 경로에 필요한
인증서를 발급하고, 프록시를 리로드한 뒤 새 설정이 제공될 때까지 기다린다.
컨테이너는 시작하지도 정지시키지도 변경하지도 않는다.

창에도 같은 동작이 설정 화면의 유지 관리에 있다. 창은 이제 실패만이 아니라 모든
결과를 로그로 남긴다. 그러지 않으면 창이 보여준 내용은 다음 동작이 덮는 순간
사라지고, 어떤 동작이 무엇을 보고했는지 되짚을 방법이 없다. `docs/operations/troubleshooting.md`가
이 동작이 답하는 증상과, 경로의 두 대상 중 어느 쪽이 응답했는지
`X-Bay-Route`로 확인하는 방법을 적는다.

검증: 이 머신에서 실행하니 경로 5개를 다시 쓰고 프록시를 리로드했으며, 이후 모든
도메인이 45ms 안에 200으로 응답하고 `x-bay-route: address`를 반환했다.
창에서 실행했을 때는 `~/.bay/conf.d/stack.conf`가 다시 쓰였음을 파일
수정 시각으로 확인했고, 창과 로그 모두 `프록시 설정을 다시 썼습니다 · 경로 5개`를
보고했다.

### 경로는 설정을 만든 그 주소로 요청을 보낸다

프록시가 재생성된 컨테이너가 더 이상 갖지 않은 주소로 연결해 요청이 시간
초과된다는 보고를 받았다. 런타임 DNS를 측정하니 원인이 나왔다. 컨테이너를
재생성한 뒤 약 15초 동안 이전 주소로 응답한다. 프록시는 백엔드를 이름으로 두고
요청마다 해석했으므로, bay이 방금 시작한 서비스가 이전 주소로 보내졌다.

이제 경로는 bay이 설정을 쓸 때 런타임에서 읽은 주소를 갖고, 요청은 그
주소로 간다. 컨테이너 이름은 두 번째 대상으로 남는다. 연결은 nginx 기본값 1분
대신 2초를 주고, 주소가 응답하지 않는 경로는 이름으로 다시 시도해 bay
없이 재생성된 컨테이너를 되찾는다. 어느 쪽이 응답했는지는 모든 응답의
`X-Bay-Route`에 적힌다.

검증: `make check`가 설정을 검사한다. 모든 `proxy_pass`에 상한이 있는지, 주소가
있는 경로가 되돌아갈 이름을 갖는지를 포함한다. 런타임에 대해서는
`BAY_E2E=1 go test ./internal/stack -run TestRecreatedServiceIsReachedAtOnceActualRuntime`가
컨테이너를 재생성하고 그에 맞춘 설정을 쓴 뒤 요청 하나를 측정했다. 주소를 통해
21ms에 응답했다. 컨테이너를 이름으로 두던 이전 설계로 같은 테스트를 돌리면 1분
0.027초 뒤 504였다. `-run TestRecreatedBehindBayRecoversActualRuntime`는
bay을 거치지 않고 재생성한 뒤 이름을 통한 복구를 2초로, 가장 오래 걸린
요청을 2.024초로 측정했다. 두 테스트 모두 자체 nginx를 띄우므로 머신에서 돌던
프로젝트는 계속 서비스한다.

### 재시작 후 현재 백엔드 인스턴스 확인

프록시 설정 generation에 라우트 컨테이너의 현재 IPv4 주소와 시작 시각을 포함한다. 재시작 명령은 해당 인스턴스의 generation을 제공하는 worker를 기다린다. DNS 백엔드 이름·캐시 시간·수명주기 제한 시간은 변경하지 않는다.

`TestBackendRestartWaitsForCurrentProxyGeneration`이 주소 또는 시작 시각 변경 후 대기가 일찍 종료되는 문제를 0.949초에 재현한다. 수정한 통합 검사와 reload 회귀는 race 3.223초에 통과한다. 전체 stack race는 3.085초에 통과하며 `make check`도 통과한다. 설치와 실제 소비자 브라우저 검증은 대기 중이다.

### 긴 라우트 이름을 위한 nginx server-name bucket 크기 지정

`RenderNginx`는 생성 HTTP 설정에 `server_names_hash_bucket_size 512`를
지정한다. 라우트 이름과 reload 동작은 그대로다. 인증서 저장은 여전히 253바이트
도메인에 필요한 파일명을 만들지 못한다.

검증: 수정 전 격리된 `TestRenderNginxLongNamesActualRuntime`이 기존 프록시
이미지의 실제 `nginx -t`로 61바이트 이름의 생성 설정을 검사했으며
`server_names_hash_bucket_size: 64` 오류로 실패했다(테스트 0.990초, 패키지
1.317초). Bucket 512로 수정한 뒤 변경하지 않은 테스트와 이미지가 통과했다
(테스트 1.010초, 61바이트 하위 테스트 0.930초, 패키지 1.309초). 테스트는 자체
컨테이너와 임시 인증서를 소유하며 공유 프록시·DNS·이미지를 변경하지 않는다.
네이티브 테스트를 끈 상태에서 `make check`가 통과했고
`go test -race ./internal/stack -count=1 -timeout=120s`가 2.200초에 통과했다.
`make -B install PREFIX=/opt/homebrew`로 설치한 바이너리는 변경 사항이 없는
소스 `9823d0e`를 기록하며 `doctor`는 `nothing to do`를 보고한다.
애플리케이션 런타임·브라우저 검증은 각 사용 프로젝트에서 기록한다.

### 프록시 reload 실패의 원래 오류 반환

`ReloadProxy`는 `container exec bay-edge nginx -s reload`를 한 번
호출한다. 명령이 실패하면 공유 프록시를 중지·시작·삭제·재생성하지 않고 nginx
stderr를 포함한 원래 오류를 반환한다.

검증: `TestReloadProxyOnlyExecutesReload`가 가짜 CLI로 오류가 사라지고 stop/start가
호출되는 현상을 먼저 재현했다. 수정한 함수의 성공·실패 경우는 race 검사를 통과했다.
`make check`가 통과했고, 네이티브 테스트를 끈 상태에서
`go test -race ./internal/stack -count=1 -timeout=120s`가 1.958초에 통과했다.
이 변경의 검증에서 설치나 실제 컨테이너 생명주기 명령은 실행하지 않았다.

### Status와 doctor가 생성·등록 없이 공개 상태를 조회

`status`는 선택한 Compose 프로젝트를 등록·갱신하지 않는다. `doctor`와 명령·창의
스냅샷은 CA 생성이나 개인 키 열기 없이 공개 인증서를 읽는다. 없거나 읽을 수 없는
공개 인증서는 JSON의 `certificates.authority.readError`를 포함해 결과에 표시한다.
`doctor`는 준비를 설명할 때 선택한 프로젝트의 도메인을 계속 포함하지만 저장하지
않는다. `up`과 명시적 설치의 준비 동작은 유지한다.

검증: 격리 CLI 회귀가 수정 전에 상태 생성과 손상된 개인 키에 의한 조회 실패를
재현했다. 수정 후에는 없는 상태와 기존 등록의 내용·모드·수정 시각을 보존하고,
잘못된 공개 인증서를 보고하며, 개인 키가 없거나 손상돼도 공개 조회를 수행한다.
네이티브 테스트를 끈 상태에서 `make check`와
`go test -race ./cmd/bay ./internal/stack ./internal/contract -count=1 -timeout=120s`가 통과했다.
이 변경의 검증에서 설치나 컨테이너 생명주기 명령은 실행하지 않았다.

### 창을 디자인 파일에서 만들고, 영어와 한국어로 표시

`design/Mockups.dc.html`이 창의 열 개 화면을 담고 치수와 색의 출처가 된다. 이
파일을 렌더해 측정한 값을 창이 그대로 사용한다. 사이드바 행 27포인트, 그룹
머리글 29, 버튼이 있는 카드 행 42와 없는 행 35, 판정 줄의 9포인트 점과 4포인트
후광, 배경 없이 테두리만 그리는 칩, 카드 외곽선과 행 사이 가는 선의 서로 다른
두 굵기다.

없던 화면 셋을 구현했다. 서비스 화면은 한 서비스의 경로, 컨테이너, 출력 끝부분을
보여준다. 설정 화면은 애플리케이션 설정, 머신 설정, 유지 관리 동작을 담는다.
컨테이너가 실행 중인데 프록시가 응답하지 않는 상태를 대시보드가 오류로 보고하고
주소를 링크로 제공하지 않는다. 도메인 추가는 창에 붙은 시트로 묻고, 입력하는 동안
만들어질 이름을 함께 보여준다. `Add project…`는 파일 패널에서 고른 Compose 파일을
등록한다.

프로젝트를 선택하면 그 서비스가 프로젝트와 같은 행 높이로 들여쓰여 나열된다.
목록이 선택을 따라가므로 펼침 컨트롤은 두지 않는다. 모든 행이 상태 점을 가지므로
정지된 서비스도 열을 유지한다.

설정에서 언어를 고른다. 시스템, English, 한국어이며 시스템은 운영체제가 선호하는
로케일을 따른다. 영어는 호출 지점에 적는 문구이고 한국어는 그것으로 찾으므로,
번역이 없는 줄은 영어로 표시된다. 선택을 바꾸면 재시작 없이 창을 다시 그린다.
명령줄은 영어를 유지한다. 그 출력이 생성 문서의 근거인 명세이기 때문이다.

리졸버 파일을 `/etc/resolver/{devel,staging,test}`처럼 묶어서 적는다. 전체 경로를
나열하면 행에 들어가지 않아 가운데가 잘리고 이름 하나가 가려졌다.

사이드바와 오른쪽 화면이 선으로 맞닿는다. 카드 외곽선과 같은 굵기다. 프로젝트
화면에서 Compose 파일을 텍스트 창에 첫 줄부터 표시하므로, 프로젝트를 편집하는
파일을 창을 벗어나지 않고 읽는다.

배치 오류 셋을 고쳤다. 위쪽 정렬 행이 두 줄짜리 값을 감싸도록 늘어나지 않아 둘째
줄이 카드 끝에서 잘렸고, 이제 값 아래 여백을 직접 명시한다. 출력 창의 한 줄이
내용 너비가 아니라 창 너비에 묶여 좌우 여백을 덮었다. 키와 값 줄은 글자가 이어져야
해서 간격이 0인데, 그 때문에 오른쪽 끝 버튼 둘이 맞붙었다. 이제 그 자리의 컨트롤만
하나씩 간격을 둔다.

검증: `make check`가 통과하며 여기에는 `go test ./internal/i18n`이 포함된다. 이
테스트는 창 소스를 읽어 한국어가 없는 줄, 영어와 값이 달라진 번역, 금지된 표현을
실패로 처리한다. 디자인 파일을 브라우저에서 렌더해 측정하고, 빌드한 창을
`screencapture -l <창>`으로 캡처해 같은 수치와 비교했으며 사이드바 행 간격이 모든
행에서 29포인트로 나왔다. 대시보드, 프로젝트, 서비스, 도메인, 인증서, 설정, 도메인
추가 시트, 첫 실행과 프록시 무응답 상태까지 모든 화면을 열어 캡처했다.

### 선택한 digest를 확인한 뒤 로컬 이미지를 시작

로컬에 있는 tag와 같은 name@digest 별칭이 없을 수 있다. 서비스는 선언한 참조로 `create`한 뒤 중지된 컨테이너의 실제 이미지·소유권·설정을 검증하고 해당 컨테이너를 시작한다. 초기화는 `start --attach`를 사용하며 실제 exit 0을 계속 요구한다. 생성 중 tag가 바뀌면 프로세스 시작 전에 거부한다. 생성 오류는 런타임 인수나 프로세스 출력을 출력하지 않고 단계와 크기가 제한된 분류를 제공한다.

검증: 실제 로컬 tag fixture는 수정 전에 실패했고 수정 후 재사용·비정상 종료 거부·생성 오류 비공개 처리·네이티브 로그의 공개 stdout marker 보존을 확인하며 통과했다. 단위 테스트는 생성된 컨테이너의 이미지·소유자·설정 변경을 거부하고 진단 저장 크기를 제한한다. 기존 서비스·준비 상태·볼륨 회귀를 유지하며 검증은 바이너리를 설치하지 않는다. 로그 저장이 금지된 값은 애플리케이션이 프로세스에 출력하지 않아야 한다.

### Compose가 관리 named volume을 선언하고 보존

관리 `volumes` 선언은 선택적 `driver_opts.size`로 없는 local 볼륨을 생성한다. 기존 볼륨은 프로젝트·선언 소유권과 정확한 설정 크기가 맞아야 하며 다른 소유자의 데이터·암묵적 크기 변경·미지원 driver 및 옵션은 거부한다. `down`은 볼륨을 보존한다. External 볼륨은 계속 존재해야 하며 생성하거나 label을 바꾸지 않는다.

검증: `make check`, `go test -race ./internal/stack ./internal/contract`, 추적된 `BAY_SERVICE_E2E=1 go test -race ./internal/stack -run TestManagedVolumeActualRuntime -count=1 -timeout=120s`가 고유한 fixture 볼륨·컨테이너만 사용해 생성·재사용·서비스 제거 뒤 데이터 보존·다른 소유자 및 크기 변경 거부를 검사한다. 기존 컨테이너와 공유 프록시는 보존하며 바이너리는 설치하지 않는다.


### Compose 시작이 소유 서비스를 보존하고 의존성을 기다림

Compose 값은 프로젝트 `.env`와 shell 치환을 해석한다. 상대 bind 경로는 Compose 디렉터리를 기준으로 하며 시작 전에 external named volume을 확인한다. `user`·`read_only`·`cap_drop`를 런타임에 전달한다. 의존성 순환·없는 서비스·미지원 생명주기 옵션은 거부한다.

`up`은 변경 없는 설정과 로컬 이미지 digest를 재사용해 실행 중 DB를 보존한다. 다른 프로젝트의 소유 label 또는 소유 label이 없는 컨테이너는 변경 전에 거부한다. 선언한 healthcheck와 실제 성공한 일회성 종료를 기다린다. 비공개 완료 기록은 설정·이미지·런타임 생성 및 시작 시각으로 중지한 초기화를 식별한다. 명시적 restart는 초기화를 재시도하며 없거나 바뀐 증거를 성공으로 간주하지 않는다. 프로젝트 변경은 advisory 잠금을 사용하고 종료는 의존성 역순을 지키며 볼륨을 제거하지 않는다.

검증: `make check`, `go test -race ./internal/stack ./internal/contract`, `BAY_SERVICE_E2E=1 go test ./internal/stack -run TestServiceLifecycleActualRuntime -count=1 -timeout=120s`. 격리 서비스 검사는 기존 모든 컨테이너를 보존하면서 실제 프로세스 제한·초기화·실패·변경 없는 재사용을 확인한다. 라우트 등록·공유 프록시 교체·애플리케이션 DB 실행·바이너리 설치는 하지 않는다. 런타임 식별 정보의 제한과 지원 문법은 [생명주기 명세](docs/spec/service-lifecycle.ko.md)에 기록한다.

경로 동기화 후 TCP 준비 상태 보고를 유지한다. 완료된 초기화는 제외하고 빈 선택은 조회하거나 연결 성공을 보고하지 않는다. 두 경우는 수정 전에 실패했고 tracked lifecycle 테스트에서 통과한다. 공유 프록시를 사용하는 기존 준비 상태 테스트는 변경 없이 유지하며 해당 프록시 사용 중에는 건너뛴다. `TestReadinessActualRuntime`은 격리 서비스로 두 snapshot 경로·지연된 listening·WaitReady를 검증한다.

## 2026-09-07

### running과 구분되는 starting 상태

컨테이너는 안의 프로세스가 포트를 듣기 전에 running으로 보고된다. 그 구간의
요청은 실패한다. 프록시가 502를 반환하고 다른 서비스의 연결은 시간 초과된다.
도구는 그런 서비스를 running으로 보고했다.

`status`, 스냅샷, 앱이 이제 `starting`으로 보고하고, `up`, `start`, `restart`는
프로세스가 연결을 받을 때까지 최대 20초 기다리며 더 걸리는 서비스의 이름을
표시한다.

검증: `BAY_E2E=1 go test ./internal/stack -run
TestStartingIsDistinctFromRunning`. 25초 뒤에 듣는 서비스를 가진 프로젝트가
`still starting after 20s`를 출력했고, `status`가 그 서비스를 `starting`으로
표시하는 동안 프록시가 502를 반환했다.

### 사용 문서를 재작성하고 실행으로 검증

`docs/operations/using.md`와 한국어 문서가 머신 확인, 프로젝트 추가, 서비스 이름
결정, 도메인 없는 서비스, 서비스 간 주소, 포트 결정, 명령줄에서의 도메인 관리,
일상 명령, 요청이 처리되는 경로를 다룬다.

모든 주장을 문서대로 만든 프로젝트에 대해 실행했다. 문서에 없던 동작 하나를
발견해 추가했다. 재시작한 서비스는 다른 서비스가 이름으로 도달하기까지 몇 초가
걸린다. 런타임이 컨테이너 실행 뒤에 새 주소를 게시하기 때문이다.

검증: 그 프로젝트에 대해 `bay up`, `stop`, `start`, `status --json`,
HTTP 리다이렉트, 이름을 통한 서비스 간 접근, 다른 프로젝트가 제공하는 도메인의
거부를 각각 실행했다.

### 명령줄에서 도메인을 관리

`bay domain`이 머신의 도메인을 나열하고 `add`, `remove`, `default`가
변경한다. 지금까지는 앱에서만 바꿀 수 있어 명령줄만 있는 사용자는 도메인을 설정할
수 없었다.

적용에 실패하면 설정 파일을 되돌린다. 인증이 거부돼도 도메인이 기록만 되고
위임되지 않는 상태가 남지 않는다.

`bay brief`가 첫 단계, `up`이 하는 일, 완전한 Compose 예제, 도메인 명령을
담아 명령 하나로 계약 전체를 다룬다.

검증: `bay domain`이 위임된 도메인을 출력한다. 이미 있는 도메인 추가,
기본 도메인 삭제, 잘못된 동작을 각각 이름을 대며 거부한다. 인증이 거부된 뒤
`~/.bay/machine.json`이 그대로다.

### 설치

`make install`은 빌드된 바이너리를 `PREFIX/bin`에, 앱을 `APPDIR`에 복사하고
`make uninstall`이 제거한다. 이전의 `install` 대상은 머신 설정을 실행했는데,
그것은 `bay install`이 이미 한다.

명령줄은 공개 모듈에서 `go install`로도 설치할 수 있다. 클론이 필요 없다.

미리 빌드한 다운로드는 제공하지 않는다. 격리된 서명 없는 바이너리는 실행 전에
종료되고, 서명하려면 Apple Developer ID가 필요하다.

검증: 빈 GOBIN에 두 명령을 `go install`해 동작하는 바이너리를 얻었고, 공개
저장소를 클론해 `make`로 빌드했으며, 임시 접두사로 `make install`을 실행해
바이너리와 앱을 설치하고 설치된 앱이 새 위치에서 실행됐으며 `make uninstall`이 두
디렉터리를 비웠다. `com.apple.quarantine`을 붙인 릴리스 아카이브는 137로
종료됐다.

### 외형 전환을 양방향으로 검증

시스템 외형이 Dark인 상태에서 실행 중인 창에서 Light를 선택하니 창이 라이트로
바뀌었고 `defaults read dev.bay.bar appearance`가 `light`를 반환했다.
저장된 값은 다음 시작에 적용된다.

검증: 선택 후 `defaults read`, 그리고 창 캡처.

### 실행 중인 프로젝트의 도메인을 다른 프로젝트가 가져가지 않음

실행 중인 프로젝트가 제공하던 도메인을 새 프로젝트가 요구하면 라우트를 빼앗고
경고만 출력했다. `up`과 `start`는 컨테이너를 만들기 전에 거부하고 도메인을
보유한 프로젝트를 명시한다.

이 결함은 `docs/operations/using.ko.md`를 그대로 따라 하다 발견했다. 기본
도메인을 쓰는 새 프로젝트가 실행 중인 프로젝트의 `web.test`를 가져갔다.

검증: `BAY_E2E=1 go test ./internal/stack -run
TestUpRefusesADomainAnotherProjectServes`, 그리고 이후 운영 문서를 처음부터 끝까지
실행했다.

### 종단 테스트가 자기 라우트만 단언

테스트가 머신 전체의 라우트 수를 단언해, 다른 프로젝트가 실행 중이면 실패했다.
각 테스트가 자기 도메인 아래의 라우트만 세도록 변경했다.

`docs-check`는 실행한 명령을 명시하지 않은 기능 행을 거부하고, 문서가 코드에
선언되지 않은 명령, Compose 키, 라벨을 언급하는지 검사한다. 이전의 빈 칸 검사는
비어 있지 않은 아무 문장이나 통과시켰다.

검증: 다른 프로젝트 두 개가 실행 중인 상태에서
`BAY_E2E=1 BAY_E2E_FORCE=1 go test ./internal/stack -count=1`이
통과하고, `make check`가 통과한다.

### 문서 구조 재편

문서를 주제별로 나눴다. `docs/spec/`은 계약, `docs/operations/`은 절차,
`docs/features.md`는 구현 상태와 근거, `CHANGELOG.md`는 동작 변경을 담는다.
`README.md`가 이들을 연결한다. `GUIDE.md`는 코드가 더 이상 읽지 않는 프로젝트
형식을 설명하고 있어 삭제했다.

독자용 문서마다 `.ko.md` 한국어 문서를 둔다. `make docs-check`가 링크, 한국어
문서, 기능 행을 검사하고 `make check`가 이를 실행한다.

`cmd/`와 `internal/`의 주석을 동작 이름, 주체와 대상, 한 문장 원인으로
재작성했다.

검증: 빌드, `go test`, `docs-check`, `go vet`을 실행하는 `make check`가
통과한다.

### 사용법을 명령으로 이동

`bay brief`, `bay schema`, `bay help <명령>`이 사용
계약, Compose 파일 계약, 명령별 효과를 출력한다. 바이너리만 설치한 프로젝트는
이 저장소를 받지 않으므로 이 출력을 받는다.

명령과 Compose 선언은 `internal/contract`에 있다. 명세 문서는 같은 선언에서
생성된 절을 담고, 문서와 코드가 다르면 `make docs-check`가 실패한다.

검증: `bay brief`, `bay schema`, `bay help up`이
출력을 생성했다. `make docs-generate`가 생성 절을 작성했고 재실행 시 변경이
없었다.

### 창을 사이드바와 대시보드 구조로 재작성

창은 Dashboard, Domains, Certificates와 프로젝트별 항목을 담은 사이드바와 선택한
항목의 상세 화면을 표시한다. 대시보드는 모든 프로젝트의 서비스 상태와 프록시가
제공하는 모든 주소를 나열한다. 타이틀바의 외형 컨트롤이 Auto, Dark, Light를
선택하고 사용자 기본 설정에 저장한다.

검증: 앱을 실행해 각 화면을 캡처했다.

### 프로젝트 형식을 Compose 파일로 교체

프로젝트는 Compose 파일이다. 설정은 `x-bay`과 `bay.*` 서비스
라벨에 있다. 이 도구가 읽지 않는 키는 무시한다.

검증: `go test ./internal/stack`이 두 가지 라벨 표기, `bay.port`,
`expose`, `ports`에서의 포트 결정, 두 형식의 command와 entrypoint, 디렉터리
검색을 검사한다. internal 데이터베이스 서비스를 포함한 Compose 파일로 프로젝트를
생성해 실행했다.

### 도메인을 머신 상태로 변경

도메인은 머신당 한 번 위임되고 `~/.bay/machine.json`에 기록된다.
프로젝트는 Compose 파일이 도메인을 지정하지 않으면 머신 기본값을 사용한다.
프로젝트가 지정한 도메인의 제거는 거부된다.

검증: `go test ./internal/stack`이 추가, 제거, 기본값 변경, 거부를 검사한다.
프로젝트의 Compose 파일에서 도메인 설정을 모두 지운 뒤에도 머신 기본값으로
해석되었다.

### internal 서비스

`bay.internal` 라벨이 붙은 서비스는 도메인, 라우트, 인증서 없이
실행된다. 다른 서비스는 `<프로젝트>-<서비스>.container.test:<포트>`로 접근한다.

검증: `go test ./internal/stack`이 라벨과 도메인 병용 거부를 검사한다. 종단
테스트가 스냅샷에 라우트와 URL이 없음을 확인했고, 데이터베이스 서비스를 가진
프로젝트가 이름으로 접근했다.

### 명령이 프록시를 기다림

`nginx -s reload`는 시그널을 보낸 뒤 반환하고, 교체되는 워커가 처리하지 않을
연결을 받을 수 있다. 프록시가 `/__bay/health`에서 설정 세대를 보고하고,
명령이 반환 전에 이를 조회한다.

검증: 15초 클라이언트 시간 초과로 실패하던 종단 테스트가 4초 이내에 완료되었다.

### 종단 테스트가 실행 중인 머신을 건드리지 않음

프록시는 머신 단위이므로, 다른 상태 디렉터리를 쓰는 테스트가 사용자가 실행 중인
프록시를 제거했다. 이제 다른 상태 디렉터리의 프록시가 실행 중이면 테스트를
건너뛴다. `BAY_E2E_FORCE`를 설정하면 실행한다. `EnsureProxy`는 마운트된
디렉터리가 다른 프록시를 재생성한다.

검증: `~/.bay`의 프록시가 실행 중인 상태에서 종단 테스트가 상태
디렉터리를 명시하며 건너뛰었다.

### 프록시가 없을 때 DNS가 SERVFAIL을 반환

DNS 서버는 프록시가 없을 때 NXDOMAIN을 반환했고 macOS가 이를 캐시해, 프록시가
돌아온 뒤에도 이름이 해석되지 않았다. 캐시되지 않는 SERVFAIL을 반환하도록
변경했다.

검증: 프록시를 정지한 상태에서 질의가 SERVFAIL을 반환했다.

### 관리자 권한 없이 인증 기관 신뢰

인증 기관 신뢰는 관리자 인증서 저장소를 사용했는데, 이는 root가 필요하고 해당
작업에 필요한 확인을 표시할 수 없다. 사용자 신뢰 설정을 사용하도록 변경했다.

검증: 관리자 권한 없이 기관에 대한 `security add-trusted-cert -r trustRoot`가
성공했고, `security verify-cert`가 인증서를 유효하다고 보고했다.

### down은 실행 중인 모든 컨테이너를 먼저 중지한다

`bay down`은 `stop_grace_period`가 없는 서비스를 `container rm --force`로 제거했고, 이는 컨테이너를 강제 종료한다. 이제 실행 중인 각 컨테이너를 Compose와 같이 제거 전에 중지한다. `stop_grace_period`가 있는 서비스는 `container stop --time <초>`로 중지하고, 나머지 서비스는 런타임 기본값으로 중지한다. 중지가 실패하면 오류를 반환하고 컨테이너를 남긴다.

검증: `TestDownStopsEveryRunningContainerBeforeRemoving`은 변경 전에 `app-cache` 제거 앞에 중지가 없어 실패했고, 변경 후 통과한다. `make check`가 통과한다. 변경은 실제 컨테이너에 대해 실행하지 않았다.
