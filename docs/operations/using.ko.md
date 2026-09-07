# bay 사용

프로젝트를 추가하고, 실행하고, 일상적으로 쓰는 명령이다.

## 프로젝트 추가

프로젝트는 Compose 파일이다. 추가 설정이 없는 서비스는
`<서비스>.<머신 기본 도메인>`으로 제공된다.

```yaml
name: shop

services:
  web:
    image: node:26-slim
    ports: ["3000"]
    command: ["npm", "run", "dev"]

  db:
    image: postgres:18
    expose: ["5432"]
    environment:
      POSTGRES_PASSWORD: dev
    labels:
      bay.internal: "true"
```

```sh
cd ~/work/shop
bay up
```

`https://web.test/`가 제공된다. `db`는 internal로 표시되어 도메인 없이 실행된다.

이 도구가 읽는 모든 키는 `bay schema`로 확인한다.

## 도메인

도메인은 머신당 한 번 위임된다. 프로젝트는 Compose 파일이 도메인을 지정하지
않으면 머신 기본값을 사용한다.

```yaml
x-bay:
  domain: shop.test
```

머신 도메인은 앱의 **Domains** 화면에서 추가하고 제거한다. 도메인을 추가하면
`/etc/resolver` 항목을 작성하고 암호를 한 번 묻는다.

`shop.test`처럼 라벨이 두 개인 도메인은 라우트가 없는 이름에도 유효한 와일드카드
인증서를 준다. `test`처럼 라벨이 하나인 도메인은 그렇지 않아, 오타 난 이름은 404
대신 인증서 경고를 낸다.

## 포트

컨테이너 포트는 다음 중 처음 일치하는 값에서 읽는다.

1. `bay.port` 라벨
2. `expose`의 첫 항목
3. `ports` 첫 항목의 컨테이너 쪽
4. 80

호스트 포트는 무시한다. 컨테이너마다 주소가 있으므로 호스트에 게시하지 않는다.

## 서비스 간 통신

다른 서비스는 `<프로젝트>-<서비스>.container.test:<포트>`로 접근한다.

```yaml
    environment:
      DATABASE_URL: postgres://shop-db.container.test:5432/app
```

컨테이너 주소는 시작할 때마다 바뀌므로 이름을 사용한다.

## 일상 명령

```sh
bay up                 # 프로젝트를 시작하고 라우트를 등록
bay down               # 프로젝트를 제거하고 라우트를 회수
bay restart web        # 서비스 하나
bay stop db            # 컨테이너를 남기고 라우트를 회수
bay logs -f web        # 출력을 따라간다
bay status             # 프록시와 등록된 모든 프로젝트
bay status --json      # 같은 내용을 다른 프로그램용으로
bay doctor             # 빠진 설정
```

앱도 같은 동작을 수행한다. 프로젝트 화면에 Start, Restart, Stop이 있고, 서비스
행마다 Start, Stop, Logs가 있다.

## 다른 사람에게 넘기기

Compose 파일을 커밋한다. 인증서와 키는 커밋하지 않는다. 머신마다 자기 인증
기관으로 직접 발급한다.

```sh
git clone ... && cd shop
bay up
```

프로젝트가 머신에 아직 위임되지 않은 도메인을 쓰면 첫 `up`에서 암호를 묻는다.
