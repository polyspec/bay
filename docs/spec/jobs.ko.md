# 작업(job)

상태: 구현됨.

작업은 끝나는 Compose 서비스의 한 번 실행이다. 테스트, 빌드, 마이그레이션, 보고서 같은 것이다. 기계는 작업을 한 번에 하나씩, 넣은 순서대로 돌린다. 그래서 무거운 실행 둘이 기계의 메모리를 두고 다투지 않는다. 작업은 요청이나 일정으로 큐에 들어가고, 각 작업의 출력, 종료 코드, 걸린 시간, 메모리 최고치가 남는다.

## 작업 선언

`x-bay.job: true`인 서비스가 작업이다. `up`과 `start`는 이 서비스를 건드리지 않는다. 어떤 서비스도 이 서비스에 의존할 수 없고, 이 서비스는 도메인을 제공하지 않는다.

```yaml
name: orm

services:
  mysql:
    image: mysql:8.4
    expose: ["3306"]
    environment:
      MYSQL_ROOT_PASSWORD: dev
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "127.0.0.1"]
      interval: 2s
      retries: 30

  test:
    image: rust:1
    working_dir: /src
    volumes: ["./:/src"]
    command: ["cargo", "test", "-j", "2"]
    mem_limit: 2g
    environment:
      DATABASE_URL: mysql://root:dev@orm-mysql.bay.test:3306/test
    x-bay:
      job: true
      schedule: "0 3 * * *"   # 선택: 매일 03:00, 기계 시각
      timeout: 45m            # 선택: 한 번 실행의 상한, 기본 1h
```

| 키 | 기본값 | 뜻 |
| --- | --- | --- |
| `x-bay.job` | `false` | 이 서비스는 작업이다. |
| `x-bay.schedule` | 없음 | cron 다섯 필드(분, 시, 일, 월, 요일, 기계의 현지 시각) 또는 `@hourly`, `@daily`, `@weekly`, `@monthly`, `@yearly`. 각 필드는 `*`, 숫자, `a-b`, `a,b`, `/n`을 받는다. cron과 같이 두 날짜 필드가 모두 제한되면 어느 쪽이든 맞는 날에 돈다. |
| `x-bay.timeout` | `1h` | 한 번 실행이 걸릴 수 있는 최대 시간, 최대 `24h`. |

틀린 값은 파일을 읽을 때 키 이름과 함께 거부한다.

## 한 번 실행이 하는 일

에이전트(`bayd`, macOS의 DNS 에이전트, Linux의 `bay-agent.service`)는 도는 작업이 없으면 가장 오래 기다린 작업을 잡아 `bay job run <id>`를 돌리고, 출력을 작업의 로그로 보낸다. 그 실행은 다음을 한다.

1. 프로젝트의 나머지 서비스를 `up`처럼 헬스체크와 인증서까지 갖춰 띄운다.
2. 작업 컨테이너를 만들고 붙은 채로 시작해, 출력을 나오는 대로 로그에 쓴다.
3. 컨테이너의 종료 코드, 걸린 시간, 그리고 Docker를 쓰는 Linux에서는 쓴 메모리의 최고치(컨트롤 그룹의 `memory.peak`)를 기록한다.
4. 작업 컨테이너를 지운다.
5. 작업 전에 컨테이너가 없던 프로젝트는 다시 내리고, 있던 프로젝트는 그대로 둔다.

상한에 닿았거나 취소된 작업은 `stop`이 서비스를 멈추듯 `stop_signal`과 `stop_grace_period`로 멈추고, 그래도 4와 5를 거친다. 기록은 `failed`이고 이유는 `stopped at its time limit` 또는 `cancelled`다. 실행 프로세스가 상한 뒤 5분 안에 끝나지 않으면 에이전트가 끝낸다.

실행은 자기 프로세스 그룹을 가진 별도 프로세스라 에이전트보다 오래 산다. 에이전트가 업데이트되거나 재시작해도 작업은 계속 돌고, 다음에 시작한 에이전트가 그것을 넘겨받아 기다린다(로그에 `taking over its run, process <pid>`). Linux에서는 이를 위해 에이전트 유닛에 `KillMode=process`가 있고, `bay install`이 이를 쓴다. 그 전에 쓴 유닛은 `check`가 알려 준다. 실행 중으로 기록됐는데 프로세스가 없는 작업은 `failed`(`its run ended without recording a result`)로 기록한다.

## 명령

```sh
bay job add test               # 실행을 큐에 넣고 ID를 출력
bay job add -key "$COMMIT" test  # 키마다 한 번만; 같은 키는 처음 ID를 출력
bay job add -wait test         # 넣고, 출력을 나오는 대로 보이고, 0 또는 1로 끝남
bay job list                   # 최근 작업 50개와 따르는 일정
bay job list --json
bay job show <id>              # 작업 하나와 출력 파일 경로
bay job logs [-f] <id>         # 출력; -f는 작업이 끝날 때까지 따라감
bay job wait [-timeout 2h] <id>  # 성공이면 0, 아니면 1
bay job cancel <id>            # 기다리는 작업은 빼고, 도는 작업은 멈춤
bay job schedule               # 이 프로젝트의 일정을 따름
bay job unschedule             # 따르지 않음
bay job list -peer odroid      # 승인한 peer의 작업을 링크로 읽음
bay job show -peer odroid <id>
bay job logs -peer odroid <id> # 출력의 끝부분
bay job wait -peer odroid <id> # 끊긴 링크 너머로 기다림
```

`-key`는 실행의 대상, 예를 들어 커밋을 이름 붙인다. 같은 키의 작업이 기다리거나 돌거나 보관된 작업 중에 끝나 있으면 다시 넣지 않는다. `add`는 그 작업의 ID를 출력하고 표준 오류에 그렇다고 쓰며, `add -wait`는 그 작업을 기다린다. 연결이 끊긴 뒤 `add`를 다시 보낸 호출자도 실행은 한 번만 얻는다.

ID는 작업 하나를 가리키는 앞부분이나 뒷부분만 줘도 된다. 모든 대기는 시작 전에 상한을 밝힌다. `-wait`, `wait`, `logs -f`는 최대 `-timeout`(주지 않으면 두 시간)까지 기다리고, 이를 표준 오류에 쓴다. 상한에 닿은 대기는 1로 끝나고 작업은 그대로 둔다. `bay job wait <id>`로 다시 기다린다.

`job add`와 `job schedule`은 에이전트가 돌지 않으면 표준 오류에 경고한다. 그때는 아무도 작업을 잡지 않기 때문이다. 무엇이 빠졌는지는 `bay check`가 알려 준다.

## 일정

`bay job schedule`은 프로젝트의 Compose 파일을 `<state>/jobs/schedules.json`에 기록한다. 그때부터 에이전트는 매분 그 파일을 읽어, 일정이 그 분을 가리키는 작업을 큐에 넣는다. 지난 작업이 아직 기다리거나 도는 중에 일정이 돌아오면 아무것도 넣지 않는다. 그래서 기계보다 빠른 일정이 큐를 채우지 않는다. 파일은 매분 다시 읽으므로 바뀐 일정은 `schedule`을 다시 돌리지 않아도 적용된다. 읽을 수 없는 파일은 `job list`에 보이고 건너뛴다.

## 보관 위치

`<state>/jobs/`에 작업마다 `<id>.json`과 출력 `<id>.log`가 있다. 끝난 작업은 최근 200개를 출력과 함께 남긴다. 큐는 `<state>/locks/`의 잠금 아래에서 바꾸므로, 동시에 돈 명령도 서로의 작업을 지킨다.

## 다른 기계의 작업

작업은 프로젝트를 가진 기계에서 돈다. 프로젝트의 파일, 볼륨, 설정이 거기 있기 때문이다. 다른 기계의 작업은 그 기계에서 넣는다.

```sh
ssh odroid bay -f /srv/orm/compose.yaml job add -key "$COMMIT" test
bay job wait -peer odroid <id>
```

기계 사이의 네트워크는 언제든 끊길 수 있고, 작업은 그것에 기대지 않는다. 일단 들어간 작업은 자기 기계에서 끝까지 돌고, 결과와 출력은 읽어 갈 때까지 거기 남는다. `wait -peer`는 5초마다 링크로 peer의 작업을 읽고, 링크가 없으면 한 번 그렇다고 말한 뒤 상한까지 다시 시도한다. 그래서 링크가 돌아오면 결과가 전달된다. ID를 출력하기 전에 `add`가 끊긴 호출자는 같은 `-key`로 다시 보내 같은 작업을 얻는다.

기계는 부하를 알리듯 승인한 peer에게 자기 작업을 알린다. 에이전트가 5초마다 peer 문서 옆에 `jobs.json`을 쓰는데, 최근 작업 50개와 최근 10개의 출력 끝부분(2 KiB)이 들어간다. 링크는 승인한 인증 기관이 발급한 인증서를 가진 클라이언트에게만 `/__bay/jobs`로 그것을 제공한다.

## 알림

macOS에서 에이전트는 이 기계나 승인한 peer(15초마다 읽음)의 작업이 끝나면 알림을 띄운다. 알림에는 프로젝트, 작업, 상태, 걸린 시간이 들어가고, 실패했으면 종료 코드나 이유도 들어간다. 링크가 끊긴 동안 끝난 peer의 작업은 링크가 돌아오면 알린다. 알린 것은 `<state>/jobs/announced.json`에 남기므로, 재시작한 에이전트도 두 번 알리지 않는다. Linux 기계는 알림을 띄우지 않고, 그 기계를 승인한 Mac이 띄운다.

## 에이전트가 쓸 때

MCP에서 `job`은 다른 명령과 같은 도구다. `args ["add", "test"]`로 넣으면 ID가 바로 돌아오고, 그다음 `args ["wait", "<id>"]`를 부른다. 또는 `args ["add", "-wait", "test"]` 한 번으로 넣고 기다린다. 결과는 작업의 출력과 0 또는 1의 종료 코드다. 작은 기계의 무거운 일은 `up`과 `exec` 대신 작업으로 돌려 차례를 기다리게 한다.
