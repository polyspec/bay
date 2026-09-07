# bay 사용

프로젝트를 추가하고, HTTPS로 제공하고, 일상적으로 쓰는 명령이다.

## 먼저 머신을 확인한다

```sh
bay doctor     # 준비된 머신이면 "nothing to do"를 출력한다
bay status     # 프록시와 이미 등록된 모든 프로젝트
bay domain     # 이 머신이 제공하는 도메인
```

`doctor`는 아무것도 바꾸지 않는다. 빠진 단계를 보고하면 `bay install`을
실행한다. [설치](install.ko.md) 참고.

`status`는 다른 프로젝트가 이미 제공 중인 도메인도 보여준다. 자기 프로젝트에는
다른 이름을 쓴다. `up`은 실행 중인 다른 프로젝트가 제공하는 도메인을 거부하고 그
프로젝트 이름을 알려준다.

## 프로젝트 추가

프로젝트는 Compose 파일이다. 추가 설정이 없는 서비스는
`<서비스>.<프로젝트 도메인>`으로 제공되고, 프로젝트 도메인은 파일이 지정하지
않으면 머신 기본값이다.

```yaml
name: myapp

services:
  web:
    image: node:26-slim
    ports: ["3000"]
    command: ["npm", "run", "dev"]
    volumes:
      - ./src:/app/src
```

```sh
cd ~/work/myapp
bay up
```

`https://web.test/`가 제공된다.

`ports: ["3000"]`은 컨테이너가 듣는 포트를 뜻한다. 호스트 포트로 게시하지 않는다.
컨테이너마다 주소가 있어서 프록시가 직접 연결한다.

## 서비스가 제공될 이름 정하기

기본값은 `<서비스>.<프로젝트 도메인>`이다. 라벨로 직접 지정한다.

```yaml
  web:
    image: node:26-slim
    labels:
      bay.domain: shop.test
```

이름은 프로젝트의 도메인 아래여야 한다. 한 프로젝트의 두 서비스가 같은 이름을
가질 수 없다.

## 도메인이 없는 서비스

데이터베이스, 큐, 워커는 internal로 표시한다. 도메인도 라우트도 인증서도 받지
않는다.

```yaml
  db:
    image: postgres:18
    expose: ["5432"]
    environment:
      POSTGRES_PASSWORD: dev
    labels:
      bay.internal: "true"
```

컨테이너는 실행되고 다른 서비스가 이름으로 접근한다.

## 서비스 간 통신

다른 서비스는 `<프로젝트>-<서비스>.container.test:<포트>`로 접근한다.

```yaml
  web:
    environment:
      DATABASE_URL: postgres://myapp-db.container.test:5432/app
```

컨테이너 주소는 DHCP로 할당되어 시작할 때마다 바뀐다. 이름을 사용한다. 프록시와
서비스가 요청마다 해석한다.

재시작한 서비스는 이름으로 다시 도달하기까지 몇 초가 걸린다. 런타임이 새 주소를
게시할 때까지 다른 서비스의 연결은 시간 초과로 실패한다. 프록시를 통한 요청은
영향을 받지 않는다. `start`, `stop`, `restart`는 프록시가 새 라우트를 제공한 뒤에
반환한다.

## 포트

컨테이너 포트는 다음 중 처음 일치하는 값에서 읽는다.

1. `bay.port` 라벨
2. `expose`의 첫 항목
3. `ports` 첫 항목의 컨테이너 쪽
4. 80

`ports`의 호스트 쪽은 무시한다.

## 도메인

도메인은 머신당 한 번 위임된다. 목록은 명령줄에서 관리한다.

```sh
bay domain                    # 목록
bay domain add lab.test       # 추가. 암호를 묻는다
bay domain default lab.test   # 프로젝트가 지정하지 않을 때 쓰는 도메인
bay domain remove lab.test
```

도메인을 추가하면 `/etc/resolver/<도메인>`을 작성한다. 이 파일이 그 도메인 아래
이름을 이 도구로 해석하라고 macOS에 알린다. 이 쓰기에 관리자 권한이 필요하므로
도메인마다 한 번 암호를 묻는다. `remove`는 기본 도메인과 프로젝트가 지정한
도메인을 거부한다.

프로젝트가 자기 도메인을 쓰려면 Compose 파일에 지정한다.

```yaml
x-bay:
  domain: myteam.test
```

그 프로젝트의 서비스는 `<서비스>.myteam.test`로 제공된다.

`myteam.test`처럼 라벨이 두 개인 도메인은 라우트가 없는 이름도 포함해 404를
반환한다. `test`처럼 라벨이 하나인 도메인은 그렇지 않다. 상위가 라벨 하나인
와일드카드를 클라이언트가 거부하므로, 오타 난 이름은 인증서 경고를 낸다.

앱의 **Domains** 화면도 같은 동작을 수행한다.

## 일상 명령

```sh
bay up                 # 프로젝트를 시작하고 라우트를 등록
bay down               # 프로젝트를 제거하고 라우트를 회수
bay stop db            # 서비스를 정지. 컨테이너는 남는다
bay start db           # 다시 시작
bay restart web
bay logs -f web        # 서비스 하나의 출력을 따라간다
bay status             # 프록시와 등록된 모든 프로젝트
bay status --json      # 같은 내용을 다른 프로그램용으로
```

앱도 같은 동작을 수행한다. 프로젝트 화면에 Start, Restart, Stop이 있고, 서비스
행마다 Start, Stop, Logs가 있다.

## 요청이 처리되는 경로

```
브라우저
  → /etc/resolver/<도메인>     도메인을 로컬 DNS 서버로 위임
  → bayd               프록시 주소를 반환
  → bay-edge:443      Host 헤더로 서비스를 선택
  → <프로젝트>-<서비스>.container.test
  → 컨테이너
```

프록시 하나가 머신 전체를 제공하고 모든 프로젝트가 공유한다. 설정은 실행 중인
컨테이너의 라벨에서 생성되므로, 프로젝트를 제거하면 파일을 고치지 않아도 라우트가
사라지고, 두 프로젝트가 서로의 설정을 덮어쓸 수 없다.

80 포트의 평문 HTTP는 HTTPS 주소로 308을 반환한다.

## 실패했을 때

[문제 해결](troubleshooting.ko.md)에 증상별 원인과 확인 명령이 있다. 대부분은 셋
중 하나다. 이름이 해석되지 않으면 DNS 에이전트가 등록되지 않은 것이고, 502는
컨테이너가 해석된 포트를 듣지 않는 것이며, 404는 그 이름을 가진 라우트가 없는
것이다.

## 다른 사람에게 넘기기

Compose 파일을 커밋한다. 인증서와 키는 커밋하지 않는다. 머신마다 자기 인증
기관으로 발급한다.

```sh
git clone ... && cd myapp
bay up
```

프로젝트가 머신에 아직 위임되지 않은 도메인을 쓰면 첫 `up`에서 암호를 묻는다.
