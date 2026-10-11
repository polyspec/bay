# 체크리스트

[English](checklist.md)

- [o] CT-1 실행 중인 소유 서비스를 교체 전에 중지한다. 우선순위: 서비스 교체의 정확성. 완료 기준: 추적하는 RED가 강제 옵션 없는 제거 전 중지, 이미 중지된 서비스 제거, 중지 실패 후 제거 미실행을 검증한다. 소유 도구 검사와 소비 엣지를 재시작하지 않는 실제 HTTPS 교체 검증을 실행한다. 기존 볼륨과 무관한 프로젝트를 보존한다.

CT-1은 수명주기 성공 전에 Apple 네이티브 서비스 이름이 실행 중인 컨테이너 주소와 같아야 한다. 네이티브 이벤트 구독과 설치는 CT-1-1에서 검증한다. 문서 요청은 재시도하지 않는다.

완료 근거: 강제 제거와 의존 서비스 시작의 추적 RED 사례, 교체·호스트 이름 GREEN 사례, `make check`, 엣지나 DNS 재시작 없는 두 서비스 교체 후 실제 HTTPS 요청. CLI는 변경 없는 소스 052e567에서 설치했고 설치 후 주소·HTTPS 검증이 통과했다.

  - [o] CT-1-1 반복 DNS 조회를 네이티브 주소 이벤트로 대체한다. 우선순위: 주소 일치 기준을 변경하지 않고 이벤트 사용을 준수한다. 근거: DNSServiceGetAddrInfo가 실제 서비스 교체 중 주소 삭제와 추가 이벤트를 전달한다. 완료 기준: 한 번 구독하고 주소 추가·삭제를 처리하며 준비 완료·오류·기한 만료 시 구독을 취소하고 정리를 수집한다. 소유 RED·GREEN과 엣지·DNS 초기화 없는 실제 HTTPS 교체를 실행한다. CLI 설치는 별도로 기록한다. 의존: CT-1.

CT-1-1 완료 근거: 한 번 구독의 RED·GREEN, 네이티브 C·헤더 빌드 입력의 RED·GREEN, 주소 추가·삭제와 취소 사례, `make check`, 관련 동시성 검사, 엣지·DNS 재시작 없이 두 서비스 교체 후 실제 HTTPS 요청. 네이티브 이벤트 CLI 설치는 별도 배포 기록으로 남긴다.

  - [o] CT-1-1-1 네이티브 이벤트 CLI 배포를 기록한다. 완료 기준: 설치 실행 파일의 변경 없는 구현 리비전을 확인하고 엣지나 DNS 재시작 없이 두 서비스의 실제 주소·HTTPS 교체 사례를 실행한다. 근거: 설치 CLI 리비전 `95d73d4`; 두 사례 통과. DNS 실행 파일과 메뉴 앱은 변경하지 않았다. 의존: CT-1-1.

  - [o] CT-1-1-2 호스트 이름 준비 검사가 컨테이너가 존재하기 전에 cache된 negative answer에 의존하지 않게 한다. 우선순위: 컨테이너가 존재하기 전에 이름을 조회한 새 서비스에서 `up`이 60초 후 실패했다. 원인: runtime DNS는 아직 없는 이름에 SOA 없는 NXDOMAIN을 반환하고, system resolver는 그 응답을 77초 동안 유지하며, DNSServiceGetAddrInfo 구독은 그 항목에서 응답받고 negative answer를 받지 않는다. 컨테이너는 같은 cache에서 응답하는 network gateway를 통해 조회하므로 컨테이너도 그 이름을 조회하지 못한다. 완료 기준: 준비 검사가 질문을 runtime DNS server로 보내고, server의 응답이 cache 항목을 대체하며, 준비 기한은 변경하지 않는다. 근거: RED `BAY_E2E=1 go test ./internal/stack -run TestNativeNameSubscriptionReplacesCachedNegativeAnswer`가 20초 한도 후 `received []`로 실패했다. GREEN: long-lived DNSServiceQueryRecord query에서 주소가 10ms 후 도착하고 그 뒤 system resolver가 그 주소를 반환한다. `make check`. 의존: CT-1-1.

  - [o] CT-1-1-2-1 CT-1-1-2의 CLI 배포를 기록한다. 완료 기준: 설치 실행 파일이 변경 없는 구현 리비전을 보고한다. 근거: 변경 없는 소스 `e57c129`에서 `make install PREFIX=/opt/homebrew`; `go version -m /opt/homebrew/bin/bay`이 리비전 `e57c129`와 `vcs.modified=false`를 보고한다. SHA-256 `702b4593a427ae19a1d6caf10cfe0d745abefea307646eddd8b95f0dbd48bdb7`. 실행 중인 DNS agent는 재시작하지 않았다. 의존: CT-1-1-2.
  - [o] CT-1-1-3 proxy가 route하는 project의 service보다 proxy를 먼저 시작한다. 우선순위: proxy가 멈춘 상태에서 web service가 시작할 때 proxy 주소를 읽는 project의 `up`이 실패했다: service가 먼저 시작되어 `bay-edge.container.test` 조회가 주소를 찾지 못하고 끝났으며, hostname 준비 검사가 `received []`로 실패했다. 원인: `up`은 service를 시작한 뒤 route 게시에서만 proxy를 만들었다. 완료 기준: route된 service가 있는 project의 `up`은 service를 시작하기 전에 service engine의 proxy를 route가 없어도 실행한다. 근거: RED `down` 뒤 proxy를 멈추고 시작할 때 proxy를 조회하는 web service의 project에 `bay up`을 실행하면 `hostname readiness failed: context deadline exceeded; received []`로 실패했다. GREEN 변경 후 같은 순서가 통과한다. `TestRoutedNamesAProjectWithADomain`이 조건을, opt-in `TestUpStartsTheProxyBeforeTheServicesItRoutesTo`가 시작 순서를 검사한다. `make check`. 의존: CT-1-1.
  - [o] CT-1-1-3-1 CT-1-1-3의 CLI 배포를 기록한다. 완료 기준: 설치 실행 파일이 변경 없는 구현 리비전을 보고한다. 근거: 변경 없는 소스 `6154437`에서 `make clean`과 `make install PREFIX=/opt/homebrew`; `go version -m /opt/homebrew/bin/bay`이 리비전 `6154437`과 `vcs.modified=false`를 보고한다. SHA-256 `7217c0ebb8cf5e2867442247c3149ff7f623225c13f23adf19ba41ba76e9ea0c`. `down` 뒤 proxy를 멈춘 상태에서 CT-1-1-3에서 실패한 project의 `bay up`이 통과하고 service가 연결을 받는다. 의존: CT-1-1-3.
- [o] CT-2 Compose `stop_grace_period`로 서비스를 중지한다. 우선순위: limepie 플랫폼의 개발 컨테이너는 `container stop`의 기본값 5초가 아니라 운영 유예 기간 30초로 중지해야 한다. 원인: 모든 중지 경로가 `--time` 없이 `container stop`을 실행했고, bay은 `stop_grace_period`를 읽지 않았다. 완료 기준: 이 도구는 각 서비스의 `stop_grace_period`(`30s`, `1m30s` 같은 Compose 기간)를 읽고, 잘못된 값은 서비스와 값을 함께 이름으로 보고하며 거부한다. 중지, 재시작, 교체와 실패한 초기화의 중지는 값을 초 단위 정수로 `container stop --time <초>`에 전달한다. `down`은 기록된 기간이 있는 실행 중 컨테이너를 `--time`으로 중지한 뒤 제거하며, 중지가 실패하면 컨테이너를 남긴다. 키가 없는 서비스는 `--time`을 받지 않고 `down`은 예전과 같이 제거한다. 근거: RED `go test ./internal/stack -run 'TestStopGraceRefusesAnInvalidValue|TestStopGraceIsPassedAsWholeSeconds|TestStopServicesPassesTheRecordedGrace|TestDownStopsAKeyedContainerBeforeRemoving|TestDownKeepsTheContainerWhenItsStopFails'`는 잘못된 값을 받아들였고, 기대한 `["stop" "--time" "30" "app-web"]` 대신 `stop calls = [["stop" "app-web"]]`를 보였으며, `down`은 중지를 호출하지 않았고 거부된 중지 뒤에도 nil을 반환했다. GREEN은 변경 후 같은 명령이 통과한다. 키가 없는 서비스가 설정 식별에 필드를 추가하지 않는지는 `TestUnsetGraceIsNotInTheConfigIdentity`가 검사한다. `make check`. `container stop`은 `--time <초>`(`-t`)를 받으며 기본값은 5다.
