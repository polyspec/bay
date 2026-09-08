# 기능

기능마다 한 행이다. 상태는 구현 상태, 근거는 현재 코드에 대해 실행한 검증,
포함은 `bin/` 아래 바이너리에 들어 있는지를 뜻한다.

이전 검증은 변경된 코드의 근거가 되지 않는다. 기능이 바뀌면 그 변경에서 실행한
검증으로 근거를 교체한다.

`make -B install PREFIX=/opt/homebrew`로 소스 `ea95bf0`을 설치했다. 설치한 CLI는 변경 사항이 없는 해당 리비전을 정확히 기록한다. 직접 실행한 `doctor`는 `nothing to do`를 반환하며 status는 기존 CA 신뢰·현재 DNS 에이전트·프록시 응답을 확인한다. 설치는 서비스를 재시작하거나 머신 설정을 변경하지 않고 바이너리를 복사한다.

| 기능 | 상태 | 근거 | 포함 |
| --- | --- | --- | --- |
| Status와 doctor가 머신 상태를 변경하지 않음 | 구현됨 | `go test ./cmd/bay -run 'Test(Status\|Doctor\|Queries)'`가 격리 fixture로 없는 상태·등록의 내용·모드·수정 시각 보존, 읽지 못한 공개 인증서 보고, 없거나 손상된 개인 키의 조회 성공을 검증 | 설치: `ea95bf0` |
| 프로세스 시작 전에 로컬 이미지 tag 검증 | 구현됨 | `TestLocalImageTagActualRuntime`이 digest 별칭 누락을 재현하고 로컬 tag 초기화·재사용·비공개 실패 출력·네이티브 로그 보존을 검증하며 단위 테스트가 시작 전 이미지·소유자·설정 변경을 거부 | 설치: `ea95bf0` |
| Compose가 관리하는 영속 named volume | 구현됨 | `make check`와 `go test -race ./internal/stack`이 소유권·선언 검증을 검사하고 `BAY_SERVICE_E2E=1 go test -race ./internal/stack -run TestManagedVolumeActualRuntime`가 서비스 제거·재생성 뒤 데이터·볼륨 식별 정보 보존을 확인 | 설치: `ea95bf0` |
| Compose 런타임 제한 및 순서·재사용을 지키는 서비스 시작 | 구현됨 | `make check`가 치환·mount·의존성 오류·소유권·재사용·완료 증거를 검사하고 `BAY_SERVICE_E2E=1 go test ./internal/stack -run TestServiceLifecycleActualRuntime`가 실제 제한·초기화·기존 컨테이너 보존을 검사 | 설치: `ea95bf0` |
| Compose 파일을 프로젝트 형식으로 사용 | 구현됨 | `go test ./internal/stack`과 `go test ./internal/contract`이 키 파싱, 두 가지 라벨 표기, 포트 결정, 파일 검색, 문서의 예제를 검사 | 예 |
| 머신 단위 도메인과 프로젝트 지정 | 구현됨 | `go test ./internal/stack`이 추가, 제거, 기본값 변경, 지정된 도메인 제거 거부를 검사 | 예 |
| 컨테이너 라벨에서 라우트 생성 | 구현됨 | `BAY_E2E=1 go test ./internal/stack -run TestTwoGroupsShareOneProxy`이 프로젝트 두 개를 띄워 각자 도메인으로 응답함을 확인 | 예 |
| 프로젝트가 공유하는 단일 프록시 | 구현됨 | `BAY_E2E=1 go test ./internal/stack -run TestTwoGroupsShareOneProxy`이 한 프로젝트를 내려도 다른 프로젝트가 계속 제공됨을 확인 | 예 |
| 컨테이너 주소 변경 추적 | 구현됨 | `BAY_E2E=1 go test ./internal/stack -run TestServiceRestartKeepsRoute`이 서비스를 재생성한 뒤 프록시가 새 주소에 도달함을 확인 | 예 |
| 프록시가 새 설정을 제공한 뒤 명령이 반환 | 구현됨 | `BAY_E2E=1 go test ./internal/stack`이 15초 클라이언트 시간 초과로 실패하던 자리에서 4초 이내에 완료 | 예 |
| 도메인 없는 internal 서비스 | 구현됨 | `go test ./internal/stack`이 라벨을 검사하고, 종단 테스트가 라우트와 인증서가 없음을 확인 | 예 |
| 실행 중인 프로젝트의 도메인을 다른 프로젝트가 가져가지 못함 | 구현됨 | `BAY_E2E=1 go test ./internal/stack -run TestUpRefusesADomainAnotherProjectServes`이 거부 메시지가 보유 프로젝트를 명시하고 컨테이너를 만들지 않음을 확인 | 예 |
| running과 구분되는 starting 상태 | 구현됨 | `TestReadinessActualRuntime`이 격리된 listener로 두 snapshot 경로·Ready/Live·WaitReady를 검사하고 lifecycle 테스트가 완료 초기화·빈 선택을 제외함. 기존 `TestStartingIsDistinctFromRunning`은 보존하며 공유 프록시 사용 중에는 건너뜀 | 설치: `ea95bf0` |
| 서비스 단위 start, stop, restart, logs | 구현됨 | `BAY_E2E=1 go test ./internal/stack -run TestStoppingOneServiceWithdrawsOnlyItsRoute`와 `-run TestLogsReachTheCaller` | 예 |
| 인증서 발급과 재발급 | 구현됨 | `go test ./internal/stack`이 목록, 만료 보고, 삭제, 기관 교체를 검사 | 예 |
| 관리자 권한 없이 인증 기관 신뢰 | 구현됨 | 기관에 대한 `security add-trusted-cert`가 성공하고 `security verify-cert`가 신뢰를 확인 | 예 |
| 권한을 획득해 resolver 항목 작성 | 구현됨 | 앱이 시스템 인증 창을 통해 `/etc/resolver/test`를 생성 | 예 |
| 명령줄에서 도메인 관리 | 구현됨 | `bay domain`이 목록을 출력하고 `add`, `remove`, `default`를 실행함. 이미 있는 도메인 추가, 기본 도메인 삭제, 잘못된 동작을 각각 이름을 대며 거부하고, 적용 실패 시 `~/.bay/machine.json`을 되돌림 | 예 |
| 머신 상태 스냅샷 | 구현됨 | `go test ./internal/stack`이 실행 중인 프로젝트와 스냅샷을 비교 | 예 |
| 메뉴바 앱 | 구현됨 | `open -n bin/baybar.app` 실행 후 행 버튼에 합성 클릭을 보내 로그 창이 열림. 버튼에서 콜백까지의 경로를 확인 | 예 |
| 사이드바와 대시보드가 있는 창 | 구현됨 | `open -n bin/baybar.app`과 `screencapture -l <창>`으로 사이드바, 프로젝트 행, 주소 목록이 있는 대시보드를 캡처 | 예 |
| 디자인 파일에서 가져온 창 치수 | 구현됨 | `design/Mockups.dc.html`을 렌더해 측정: 사이드바 행 27, 그룹 머리글 29, 버튼이 있는 카드 행 42와 없는 행 35, 머리글 56.5. 창을 캡처해 같은 수치로 비교했고 사이드바 행 간격이 모든 행에서 29포인트로 나옴 | 예 |
| 서비스 화면 | 구현됨 | 서비스를 선택하니 경로·컨테이너·출력 카드가 실제 주소, 이미지, 컨테이너 이름, 실행 시간과 함께 표시되고 출력 창과 그 하단 줄이 나타났으며 출력이 없을 때는 `(아직 출력 없음)`이 표시됨 | 예 |
| 설정 화면 | 구현됨 | `open -n bin/baybar.app`과 `screencapture -l <창>`으로 확인: 화면 모드 컨트롤, 실행 시 창 열기 스위치, 기본 도메인, DNS 에이전트 주소와 상태 디렉터리, 유지 관리 세 행이 표시되고 두 줄짜리 행이 카드 안에 들어가는지 측정 | 예 |
| 프록시 무응답을 오류로 보고 | 구현됨 | 프록시를 정지한 스냅샷에서 붉은 판정 줄, 경로 수를 밝힌 오류 배너, 링크가 해제되고 `응답 없음`으로 바뀐 주소가 표시됨 | 예 |
| 선택한 프로젝트 아래에 서비스 나열 | 구현됨 | 프로젝트를 선택한 상태를 `screencapture -l <창>`으로 캡처하니 서비스가 들여쓰여 나열되고 사이드바 모든 행의 점 중심 간격이 29포인트로 측정됨 | 예 |
| 도메인 추가를 시트로 질문 | 구현됨 | `Add domain…`이 창에 붙은 시트를 열어 입력란, 결과 이름 미리보기, 기본 지정 스위치를 표시했고 `Cancel`이 변경 없이 닫음 | 예 |
| 창에서 프로젝트 등록 | 구현됨 | `Add project…`가 파일 패널을 열고 선택한 Compose 파일을 `stack.LoadIn`과 `Machine.Register`로 등록 | 설치: `ea95bf0` |
| 영어와 한국어로 표시되는 창 | 구현됨 | `go test ./internal/i18n`이 창 소스를 읽어 한국어가 없는 문구, 값이 달라진 번역, 금지된 표현을 실패로 처리 | 예 |
| 리졸버 파일을 묶어서 표기 | 구현됨 | `go test ./cmd/baybar -run TestResolverPaths`가 도메인 0개·1개·여러 개를 검사. 전체 경로를 나열하면 행에 들어가지 않아 가운데가 잘리고 이름 하나가 가려졌음 | 예 |
| 창에서 Compose 파일 열기 | 구현됨 | 프로젝트 화면의 `View`가 `/tmp/guidecheck/compose.yaml`을 텍스트 창에 경로를 제목으로 첫 줄부터 표시 | 예 |
| 언어 설정: 시스템, English, 한국어 | 구현됨 | 창에서 English를 고르니 기본값 데이터베이스에 `language = en`이 기록되고 창이 영어로 캡처됨. 설정을 지우고 재시작하니 이 시스템이 선호하는 한국어로 표시됨 | 예 |
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
