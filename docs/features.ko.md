# 기능

기능마다 한 행이다. 상태는 구현 상태, 근거는 현재 코드에 대해 실행한 검증,
포함은 `bin/` 아래 바이너리에 들어 있는지를 뜻한다.

이전 검증은 변경된 코드의 근거가 되지 않는다. 기능이 바뀌면 그 변경에서 실행한
검증으로 근거를 교체한다.

| 기능 | 상태 | 근거 | 포함 |
| --- | --- | --- | --- |
| 프로세스 시작 전에 로컬 이미지 tag 검증 | 구현됨 | `TestLocalImageTagActualRuntime`이 digest 별칭 누락을 재현하고 로컬 tag 초기화·재사용·비공개 실패 출력·네이티브 로그 보존을 검증하며 단위 테스트가 시작 전 이미지·소유자·설정 변경을 거부 | 로컬 빌드; 설치 안 함 |
| Compose가 관리하는 영속 named volume | 구현됨 | `make check`와 `go test -race ./internal/stack`이 소유권·선언 검증을 검사하고 `BAY_SERVICE_E2E=1 go test -race ./internal/stack -run TestManagedVolumeActualRuntime`가 서비스 제거·재생성 뒤 데이터·볼륨 식별 정보 보존을 확인 | 로컬 빌드; 설치 안 함 |
| Compose 런타임 제한 및 순서·재사용을 지키는 서비스 시작 | 구현됨 | `make check`가 치환·mount·의존성 오류·소유권·재사용·완료 증거를 검사하고 `BAY_SERVICE_E2E=1 go test ./internal/stack -run TestServiceLifecycleActualRuntime`가 실제 제한·초기화·기존 컨테이너 보존을 검사 | 로컬 빌드; 설치 안 함 |
| Compose 파일을 프로젝트 형식으로 사용 | 구현됨 | `go test ./internal/stack`과 `go test ./internal/contract`이 키 파싱, 두 가지 라벨 표기, 포트 결정, 파일 검색, 문서의 예제를 검사 | 예 |
| 머신 단위 도메인과 프로젝트 지정 | 구현됨 | `go test ./internal/stack`이 추가, 제거, 기본값 변경, 지정된 도메인 제거 거부를 검사 | 예 |
| 컨테이너 라벨에서 라우트 생성 | 구현됨 | `BAY_E2E=1 go test ./internal/stack -run TestTwoGroupsShareOneProxy`이 프로젝트 두 개를 띄워 각자 도메인으로 응답함을 확인 | 예 |
| 프로젝트가 공유하는 단일 프록시 | 구현됨 | `BAY_E2E=1 go test ./internal/stack -run TestTwoGroupsShareOneProxy`이 한 프로젝트를 내려도 다른 프로젝트가 계속 제공됨을 확인 | 예 |
| 컨테이너 주소 변경 추적 | 구현됨 | `BAY_E2E=1 go test ./internal/stack -run TestServiceRestartKeepsRoute`이 서비스를 재생성한 뒤 프록시가 새 주소에 도달함을 확인 | 예 |
| 프록시가 새 설정을 제공한 뒤 명령이 반환 | 구현됨 | `BAY_E2E=1 go test ./internal/stack`이 15초 클라이언트 시간 초과로 실패하던 자리에서 4초 이내에 완료 | 예 |
| 도메인 없는 internal 서비스 | 구현됨 | `go test ./internal/stack`이 라벨을 검사하고, 종단 테스트가 라우트와 인증서가 없음을 확인 | 예 |
| 실행 중인 프로젝트의 도메인을 다른 프로젝트가 가져가지 못함 | 구현됨 | `BAY_E2E=1 go test ./internal/stack -run TestUpRefusesADomainAnotherProjectServes`이 거부 메시지가 보유 프로젝트를 명시하고 컨테이너를 만들지 않음을 확인 | 예 |
| running과 구분되는 starting 상태 | 구현됨 | `TestReadinessActualRuntime`이 격리된 listener로 두 snapshot 경로·Ready/Live·WaitReady를 검사하고 lifecycle 테스트가 완료 초기화·빈 선택을 제외함. 기존 `TestStartingIsDistinctFromRunning`은 보존하며 공유 프록시 사용 중에는 건너뜀 | 로컬 빌드; 설치 안 함 |
| 서비스 단위 start, stop, restart, logs | 구현됨 | `BAY_E2E=1 go test ./internal/stack -run TestStoppingOneServiceWithdrawsOnlyItsRoute`와 `-run TestLogsReachTheCaller` | 예 |
| 인증서 발급과 재발급 | 구현됨 | `go test ./internal/stack`이 목록, 만료 보고, 삭제, 기관 교체를 검사 | 예 |
| 관리자 권한 없이 인증 기관 신뢰 | 구현됨 | 기관에 대한 `security add-trusted-cert`가 성공하고 `security verify-cert`가 신뢰를 확인 | 예 |
| 권한을 획득해 resolver 항목 작성 | 구현됨 | 앱이 시스템 인증 창을 통해 `/etc/resolver/test`를 생성 | 예 |
| 명령줄에서 도메인 관리 | 구현됨 | `bay domain`이 목록을 출력하고 `add`, `remove`, `default`를 실행함. 이미 있는 도메인 추가, 기본 도메인 삭제, 잘못된 동작을 각각 이름을 대며 거부하고, 적용 실패 시 `~/.bay/machine.json`을 되돌림 | 예 |
| 머신 상태 스냅샷 | 구현됨 | `go test ./internal/stack`이 실행 중인 프로젝트와 스냅샷을 비교 | 예 |
| 메뉴바 앱 | 구현됨 | `open -n bin/baybar.app` 실행 후 행 버튼에 합성 클릭을 보내 로그 창이 열림. 버튼에서 콜백까지의 경로를 확인 | 예 |
| 사이드바와 대시보드가 있는 창 | 구현됨 | `open -n bin/baybar.app`과 `screencapture -l <창>`으로 사이드바, 프로젝트 행, 주소 목록이 있는 대시보드를 캡처 | 예 |
| 외형 전환: Auto, Dark, Light | 구현됨 | 읽기 경로: 시스템이 Dark인 상태에서 `defaults write dev.bay.bar appearance light` 후 재시작하니 창이 라이트로 렌더되고 Light가 선택됨. 쓰기 경로: 실행 중인 창에서 Light를 선택하니 창이 라이트로 바뀌고 `defaults read dev.bay.bar appearance`가 `light`를 반환 | 예 |
| 공개 모듈에서 설치 | 구현됨 | `GOBIN=/tmp/x go install github.com/polyspec/bay/cmd/bay@latest`와 `bayd`가 동작하는 바이너리를 생성했고 `bay brief`가 실행됨 | 예 |
| 클론에서 설치 | 구현됨 | 공개 저장소를 `git clone`한 뒤 `make`가 산출물 세 개를 생성하고 `bin/bay brief`가 실행됨 | 예 |
| `make install`과 `make uninstall` | 구현됨 | `make install PREFIX=/tmp/prefix APPDIR=/tmp/apps`가 바이너리 두 개와 앱을 설치했고, 설치된 앱이 새 위치에서 실행됐으며, `make uninstall`이 두 디렉터리를 비움 | 예 |
| 미리 빌드한 다운로드를 배포 경로에서 제외 | 구현됨 | `com.apple.quarantine`을 붙인 릴리스 아카이브가 실행 시 exit 137로 종료되어, 위 경로들은 머신에서 빌드하도록 함 | 예 |
| 운영 문서가 명령과 일치 | 구현됨 | `docs/operations/using.ko.md`의 주장을 문서대로 만든 프로젝트에 대해 실행: `bay up`, `stop`, `start`, `logs`, `status --json`, HTTP 리다이렉트, 이름을 통한 서비스 간 접근, 다른 프로젝트가 제공하는 도메인의 거부 | 예 |
| 명령에 담긴 사용 계약 | 구현됨 | `bay brief`, `schema`, `help <명령>`이 출력을 생성 | 예 |
| 코드에서 생성되는 명세 절 | 구현됨 | `make docs-generate`가 표를 작성하고 `make docs-check`가 비교 | 예 |
| 독자용 문서의 한국어 문서 | 구현됨 | `make docs-check`가 모든 문서가 존재하고 제목 개수가 일치함을 보고 | 예 |
| 직접적인 표현으로 작성된 코드 주석 | 구현됨 | `cmd/`와 `internal/`에서 금지된 표현을 검색해 테스트 외 일치 없음 | 예 |
