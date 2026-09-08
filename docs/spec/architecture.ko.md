# 구조

[English](architecture.md).

상태: 구현됨.

`bay`은 Apple `container`에서 Compose 프로젝트를 실행하고 각 서비스를
로컬 도메인의 HTTPS로 제공한다. 호스트 포트는 게시하지 않는다.

## 구성 요소

| 구성 요소 | 형태 | 수 |
| --- | --- | --- |
| `bay` | 명령줄 바이너리 | 필요할 때 실행 |
| `bayd` | launchd 사용자 에이전트가 시작하는 DNS 서버 | 머신당 하나 |
| `bay-edge` | TLS를 종료하는 nginx 컨테이너 | 머신당 하나 |
| `baybar` | 메뉴바 애플리케이션과 창 | 사용자 세션당 하나 |

프로젝트는 Compose 파일이다. 프로젝트는 자신의 서비스 컨테이너를 소유한다.
프록시, DNS 서버, 인증 기관, resolver 항목은 머신 단위이며 모든 프로젝트가 공유한다.

## 요청 경로

```
브라우저
  → /etc/resolver/<domain>            도메인을 127.0.0.1:5354에 위임
  → bayd                      프록시 컨테이너 주소 반환
  → bay-edge:443             Host 헤더로 server 블록 선택
  → <container>.container.test        gateway의 런타임 DNS가 해석
  → 서비스 컨테이너:<port>
```

80번 포트의 일반 HTTP는 HTTPS 주소로 308을 반환한다.
`/__bay/health`는 예외이며 설정 generation을 반환한다.

## 라우팅 상태

프록시 설정은 Compose 파일이 아니라 실행 중인 컨테이너의 라벨에서 생성한다.
`bay`은 `container ls --format json`을 읽고
`bay.role=service` 라벨이 있는 컨테이너를 선택한다.
그중 실행 중이며 `bay.domain` 라벨을 가진 컨테이너마다 nginx server
블록 하나를 작성한다.

이 규칙에 따라:

- 프로젝트 컨테이너를 제거하면 해당 라우트가 사라진다. 파일은 편집하지 않는다.
- 두 프로젝트가 서로의 설정을 덮어쓸 수 없다.
- 중지한 서비스는 다시 시작할 때까지 라우트가 없다.

## 컨테이너 주소

각 컨테이너는 `192.168.64.0/24` vmnet 서브넷의 주소를 받으며 호스트에서 접근할
수 있다. 주소는 DHCP가 할당하고 시작할 때마다 바뀐다. MAC 주소를 고정해도
IP 주소는 유지되지 않는다.

따라서 프록시는 backend를 주소 대신 이름으로 지정한다. `proxy_pass`는 변수를
사용하고 `resolver`는 네트워크 gateway를 가리키므로 nginx는 요청마다
`<container>.container.test`를 해석한다. 서비스를 재시작해도 설정 변경 없이
새 주소에 도달한다.

## 설정 다시 읽기

생성하는 HTTP 설정은 nginx의 기본 server-name bucket에 담기지 않는 긴 이름을
위해 `server_names_hash_bucket_size 512`를 지정한다. 이름과 hash
원소의 정렬·포인터 크기를 포함한 값이며 nginx의
[2의 거듭제곱 지침](https://nginx.org/en/docs/http/server_names.html#optimization)을 따른다.
`TestRenderNginxLongNamesActualRuntime`은 기존 `ProxyImage`의 `nginx -t`로
61바이트 라우팅 이름의 실제 생성 설정을 검사한다. 명시 실행 명령은
`BAY_SERVICE_E2E=1 go test ./internal/stack -run '^TestRenderNginxLongNamesActualRuntime$' -count=1 -v -timeout=120s`이다.
이미지가 이미 있어야 하며, 자체 컨테이너와 임시 인증서를 만들고 소유권을 확인한
컨테이너만 제거한다. 라우트 등록, 공유 프록시·DNS 변경, 이미지 pull·제거는 하지 않는다.

공유 프록시의 설정을 다시 읽을 때는
`container exec bay-edge nginx -s reload`를 정확히 한 번 실행한다.
명령이 실패하면 nginx stderr를 포함한 원래 오류를 호출자에게 반환한다.
reload 실패로 프록시를 중지·시작·삭제·재생성해서는 안 된다. 다른 프로젝트도
같은 프록시를 사용하므로 오류 복구를 이유로 재시작해서는 안 된다.

`nginx -s reload`는 신호를 보낸 뒤 반환한다. 교체되는 worker는 종료를 마칠 때까지
listening 소켓을 보유하며, 그동안 연결을 받아도 요청을 처리하지 않을 수 있다.

따라서 `bay`은 라우트 목록에서 generation 값을 계산해 설정에 넣고,
`http://<proxy>/__bay/health`가 그 값을 반환할 때까지 조회한다.
`up`, `down`, `start`, `stop`, `restart`는 프록시가 새 설정을 제공한 뒤 반환한다.

## 인증서

처음 사용할 때 상태 디렉터리에 인증 기관을 만들고 사용자의 신뢰 설정에 추가한다.
추가에는 관리자 권한이 필요하지 않다.

라우팅되는 도메인마다 유효 기간이 1년인 인증서를 하나 발급하며, 남은 기간이
30일보다 짧으면 재발급한다. 기본 인증서는 위임한 모든 도메인과 각 wildcard를
포함하므로 라우트가 없는 이름도 handshake를 완료하고 404를 받는다.

인증서 파일명은 도메인 뒤에 `.crt` 또는 `.key`를 붙인다. 따라서 253바이트
도메인은 파일시스템의 255바이트 파일명 한도를 넘으므로 현재 인증서 저장에서
지원하지 않는다. 61바이트 설정 회귀는 이러한 이름의 TLS 지원을 검증하지 않는다.

클라이언트는 상위 도메인이 단일 label인 wildcard를 거부하므로 `*.test`는
`nope.test`에 적용되지 않는다. `dev.test`처럼 label이 둘인 도메인의
`*.dev.test`는 허용된다.

## 도메인

`/etc/resolver`의 파일로 도메인을 머신에 한 번 위임한다. 도메인은 머신 상태이며
`~/.bay/machine.json`에 저장한다. 프로젝트는 Compose의
`x-bay.domain`으로 지정하지 않으면 머신의 기본 도메인을 사용한다.

위임 도메인 집합은 머신 도메인과 프로젝트가 고정한 도메인을 합친 것이다.

## 창

`baybar`는 소스 목록이 있는 창 하나를 표시한다. MACHINE에는 Dashboard,
Domains, Certificates, Settings가 있고 PROJECTS에는 등록한 프로젝트마다 행이
하나 있다. 프로젝트를 선택하면 그 아래에 서비스를 들여써 나열하므로 사이드바를
벗어나지 않고 서비스를 열 수 있다. 목록이 선택을 따라가므로 별도 펼침 컨트롤은 없다.

화면:

| 화면 | 대상 |
| --- | --- |
| Dashboard | 머신 전체: 상태 한 줄, 모든 프로젝트, 프록시가 제공하는 모든 주소 |
| Project | 프로젝트 하나: 서비스, 도메인, 텍스트 창으로 여는 Compose 파일 |
| Service | 서비스 하나: 라우트, 컨테이너, 출력 끝부분 |
| Domains | 머신에 위임한 도메인과 위임이 작성하는 항목 |
| Certificates | 인증 기관과 발급한 인증서 |
| Settings | 애플리케이션 설정, 머신 설정, 유지 관리 동작 |

대시보드는 머신의 세 가지 상태를 구분한다. 준비가 불완전하면 경고와 준비 동작을
제공한다. 컨테이너가 실행 중인데 프록시가 응답하지 않으면 오류로 표시하고 주소의
링크를 해제하며 이유를 적는다. 그 외에는 제공 중인 도메인 수를 표시한다.

크기, 색, 행 높이는 포인트 단위로 적힌 `design/Mockups.dc.html`에서 가져온다.
해당 파일과 의도적으로 다른 점은 다음과 같다.

- 누름 버튼은 bezel이 그리는 높이보다 작은 높이를 차지하므로 버튼을 담은 행은
  디자인의 행 높이 대신 여백을 기준으로 측정한다.
- 서비스 행은 펼침 삼각형 대신 프로젝트 아래에 들여쓰며, 상태 점은 지정 크기의
  3분의 1로 그린다.
- 키와 값 줄은 텍스트를 이어서 표시하므로 간격을 0으로 두고 끝의 컨트롤마다
  간격을 따로 지정한다.

## 언어

창 소스는 영어이며 언어 설정에 따라 한국어로 표시한다. Settings는 System,
English, 한국어를 제공한다. System은 `NSLocale.preferredLanguages`를 따르며,
선택은 defaults 데이터베이스의 `language`에 저장한다. 변경하면 재시작 없이 창을
다시 그린다. 호출 지점의 영어 문구로 한국어를 찾으므로 번역이 없으면 빈칸 대신
영어를 표시한다. `go test ./internal/i18n`은 창 소스를 읽고 한국어가 없는 문구,
영어와 받는 값이 다른 번역, 프로젝트에서 쓰지 않는 표현을 거부한다.

명령줄은 영어를 유지한다. 그 출력이 생성 문서의 근거인 명세이기 때문이다.
